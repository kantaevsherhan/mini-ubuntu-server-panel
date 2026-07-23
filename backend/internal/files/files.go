package files

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"
	"unicode/utf8"

	"github.com/kantaevsherhan/mini-ubuntu-server-panel/backend/internal/config"
)

const (
	DefaultConfigPath = "/etc/mini-ubuntu-server/config.yml"
	MaxContentBytes   = 2 * 1024 * 1024
	maxRequestBytes   = 3 * 1024 * 1024
	maxEntries        = 5000
)

type Root struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
	Path string `json:"path"`
}

type Entry struct {
	Name       string    `json:"name"`
	Path       string    `json:"path"`
	Directory  bool      `json:"directory"`
	Symlink    bool      `json:"symlink"`
	Size       int64     `json:"size"`
	Mode       string    `json:"mode"`
	ModifiedAt time.Time `json:"modified_at"`
}

type File struct {
	Path       string    `json:"path"`
	Content    string    `json:"content"`
	Size       int64     `json:"size"`
	ModifiedAt time.Time `json:"modified_at"`
}

type Controller interface {
	Roots() []Root
	List(context.Context, int, string) ([]Entry, error)
	Read(context.Context, int, string) (File, error)
	Write(context.Context, int, string, []byte) error
	Mkdir(context.Context, int, string) error
	Delete(context.Context, int, string) error
}

type Manager struct {
	Executable string
	roots      []string
}

func NewManager(roots []string) (*Manager, error) {
	executable, err := os.Executable()
	if err != nil {
		return nil, err
	}
	roots, err = config.NormalizeAllowedDirectories(roots)
	if err != nil {
		return nil, err
	}
	return &Manager{Executable: executable, roots: roots}, nil
}

func (m Manager) Roots() []Root {
	result := make([]Root, 0, len(m.roots))
	for index, root := range m.roots {
		name := filepath.Base(root)
		if name == "." || name == string(filepath.Separator) {
			name = root
		}
		result = append(result, Root{ID: index, Name: name, Path: root})
	}
	return result
}

func (m Manager) List(ctx context.Context, root int, path string) ([]Entry, error) {
	var result []Entry
	err := m.run(ctx, request{Operation: "list", Root: root, Path: path}, &result)
	return result, err
}

func (m Manager) Read(ctx context.Context, root int, path string) (File, error) {
	var result File
	err := m.run(ctx, request{Operation: "read", Root: root, Path: path}, &result)
	return result, err
}

func (m Manager) Write(ctx context.Context, root int, path string, content []byte) error {
	return m.run(ctx, request{Operation: "write", Root: root, Path: path, Content: content}, nil)
}

func (m Manager) Mkdir(ctx context.Context, root int, path string) error {
	return m.run(ctx, request{Operation: "mkdir", Root: root, Path: path}, nil)
}

func (m Manager) Delete(ctx context.Context, root int, path string) error {
	return m.run(ctx, request{Operation: "delete", Root: root, Path: path}, nil)
}

func (m Manager) run(ctx context.Context, value request, target any) error {
	if err := validateRequest(value, len(m.roots)); err != nil {
		return err
	}
	payload, err := json.Marshal(value)
	if err != nil {
		return err
	}
	command := exec.CommandContext(ctx, "/usr/bin/sudo", "-n", m.Executable, "privileged-files")
	command.Stdin = bytes.NewReader(payload)
	output, err := command.Output()
	if err != nil {
		return fmt.Errorf("privileged file operation failed: %w", err)
	}
	if target != nil {
		if err := json.Unmarshal(output, target); err != nil {
			return errors.New("invalid files helper response")
		}
	}
	return nil
}

type request struct {
	Operation string `json:"operation"`
	Root      int    `json:"root"`
	Path      string `json:"path"`
	Content   []byte `json:"content,omitempty"`
}

func RunPrivileged(input io.Reader, output io.Writer, configPath string) error {
	if os.Geteuid() != 0 {
		return errors.New("privileged-files must run as root")
	}
	roots, err := config.LoadAllowedDirectories(configPath)
	if err != nil {
		return errors.New("cannot load allowed directories")
	}
	decoder := json.NewDecoder(io.LimitReader(input, maxRequestBytes))
	decoder.DisallowUnknownFields()
	var value request
	if err := decoder.Decode(&value); err != nil {
		return errors.New("invalid file operation request")
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return errors.New("invalid file operation request")
	}
	if err := validateRequest(value, len(roots)); err != nil {
		return err
	}
	switch value.Operation {
	case "list":
		entries, err := list(roots[value.Root], value.Path)
		if err != nil {
			return err
		}
		return json.NewEncoder(output).Encode(entries)
	case "read":
		file, err := read(roots[value.Root], value.Path)
		if err != nil {
			return err
		}
		return json.NewEncoder(output).Encode(file)
	case "write":
		if err := write(roots[value.Root], value.Path, value.Content); err != nil {
			return err
		}
	case "mkdir":
		if err := mkdir(roots[value.Root], value.Path); err != nil {
			return err
		}
	case "delete":
		if err := remove(roots[value.Root], value.Path); err != nil {
			return err
		}
	}
	return json.NewEncoder(output).Encode(struct{}{})
}

func validateRequest(value request, rootCount int) error {
	if value.Root < 0 || value.Root >= rootCount {
		return errors.New("invalid file root")
	}
	if _, err := cleanRelativePath(value.Path); err != nil {
		return err
	}
	switch value.Operation {
	case "list":
		if len(value.Content) != 0 {
			return errors.New("content is not allowed for list")
		}
	case "read":
		if value.Path == "" || len(value.Content) != 0 {
			return errors.New("invalid file read request")
		}
	case "write":
		if value.Path == "" || len(value.Content) > MaxContentBytes {
			return errors.New("invalid file write request")
		}
	case "mkdir", "delete":
		if value.Path == "" || len(value.Content) != 0 {
			return errors.New("invalid file mutation request")
		}
	default:
		return errors.New("file operation is not allowed")
	}
	return nil
}

func cleanRelativePath(value string) (string, error) {
	if strings.ContainsRune(value, 0) || filepath.IsAbs(value) {
		return "", errors.New("invalid relative file path")
	}
	value = filepath.Clean(filepath.FromSlash(value))
	if value == "." {
		return "", nil
	}
	if value == ".." || strings.HasPrefix(value, ".."+string(filepath.Separator)) {
		return "", errors.New("file path escapes allowed root")
	}
	return value, nil
}

func resolve(root, relative string, finalMayBeMissing bool) (string, error) {
	root = filepath.Clean(root)
	rootInfo, err := os.Lstat(root)
	if err != nil || !rootInfo.IsDir() || rootInfo.Mode()&os.ModeSymlink != 0 {
		return "", errors.New("allowed root is unavailable or unsafe")
	}
	relative, err = cleanRelativePath(relative)
	if err != nil {
		return "", err
	}
	if relative == "" {
		return root, nil
	}
	current := root
	parts := strings.Split(relative, string(filepath.Separator))
	for index, part := range parts {
		current = filepath.Join(current, part)
		info, statErr := os.Lstat(current)
		if statErr != nil {
			if errors.Is(statErr, os.ErrNotExist) && finalMayBeMissing && index == len(parts)-1 {
				return current, nil
			}
			return "", errors.New("file path is unavailable")
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return "", errors.New("symbolic links are not allowed")
		}
		if index < len(parts)-1 && !info.IsDir() {
			return "", errors.New("file path parent is not a directory")
		}
	}
	return current, nil
}

func list(root, relative string) ([]Entry, error) {
	target, err := resolve(root, relative, false)
	if err != nil {
		return nil, err
	}
	items, err := os.ReadDir(target)
	if err != nil {
		return nil, errors.New("cannot read directory")
	}
	if len(items) > maxEntries {
		return nil, errors.New("directory contains too many entries")
	}
	base, _ := cleanRelativePath(relative)
	result := make([]Entry, 0, len(items))
	for _, item := range items {
		info, infoErr := item.Info()
		if infoErr != nil {
			continue
		}
		path := filepath.ToSlash(filepath.Join(base, item.Name()))
		result = append(result, Entry{Name: item.Name(), Path: path, Directory: info.IsDir(), Symlink: info.Mode()&os.ModeSymlink != 0, Size: info.Size(), Mode: info.Mode().String(), ModifiedAt: info.ModTime().UTC()})
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Directory != result[j].Directory {
			return result[i].Directory
		}
		return strings.ToLower(result[i].Name) < strings.ToLower(result[j].Name)
	})
	return result, nil
}

func read(root, relative string) (File, error) {
	target, err := resolve(root, relative, false)
	if err != nil {
		return File{}, err
	}
	info, err := os.Stat(target)
	if err != nil || !info.Mode().IsRegular() || info.Size() > MaxContentBytes {
		return File{}, errors.New("file is not a supported text file")
	}
	data, err := os.ReadFile(target)
	if err != nil || !utf8.Valid(data) || bytes.IndexByte(data, 0) >= 0 {
		return File{}, errors.New("file is not valid UTF-8 text")
	}
	return File{Path: filepath.ToSlash(relative), Content: string(data), Size: info.Size(), ModifiedAt: info.ModTime().UTC()}, nil
}

func write(root, relative string, content []byte) error {
	if len(content) > MaxContentBytes || !utf8.Valid(content) || bytes.IndexByte(content, 0) >= 0 {
		return errors.New("file content must be UTF-8 text")
	}
	target, err := resolve(root, relative, true)
	if err != nil {
		return err
	}
	parent := filepath.Dir(target)
	parentInfo, err := os.Stat(parent)
	if err != nil || !parentInfo.IsDir() {
		return errors.New("file parent is unavailable")
	}
	mode := os.FileMode(0640)
	ownerInfo := parentInfo
	if info, statErr := os.Stat(target); statErr == nil {
		if !info.Mode().IsRegular() {
			return errors.New("target is not a regular file")
		}
		mode = info.Mode().Perm()
		ownerInfo = info
	} else if !errors.Is(statErr, os.ErrNotExist) {
		return errors.New("cannot inspect file target")
	}
	temporary, err := os.CreateTemp(parent, ".mini-panel-*")
	if err != nil {
		return errors.New("cannot create temporary file")
	}
	temporaryPath := temporary.Name()
	defer func() { _ = os.Remove(temporaryPath) }()
	if err := temporary.Chmod(mode); err != nil {
		_ = temporary.Close()
		return errors.New("cannot set file permissions")
	}
	if stat, ok := ownerInfo.Sys().(*syscall.Stat_t); ok {
		if err := temporary.Chown(int(stat.Uid), int(stat.Gid)); err != nil {
			_ = temporary.Close()
			return errors.New("cannot preserve file ownership")
		}
	}
	if _, err := temporary.Write(content); err != nil {
		_ = temporary.Close()
		return errors.New("cannot write file")
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return errors.New("cannot sync file")
	}
	if err := temporary.Close(); err != nil {
		return errors.New("cannot close file")
	}
	if err := os.Rename(temporaryPath, target); err != nil {
		return errors.New("cannot replace file")
	}
	return syncDirectory(parent)
}

func mkdir(root, relative string) error {
	target, err := resolve(root, relative, true)
	if err != nil {
		return err
	}
	parentInfo, err := os.Stat(filepath.Dir(target))
	if err != nil || !parentInfo.IsDir() {
		return errors.New("directory parent is unavailable")
	}
	if err := os.Mkdir(target, 0750); err != nil {
		return errors.New("cannot create directory")
	}
	if stat, ok := parentInfo.Sys().(*syscall.Stat_t); ok {
		if err := os.Chown(target, int(stat.Uid), int(stat.Gid)); err != nil {
			_ = os.Remove(target)
			return errors.New("cannot preserve directory ownership")
		}
	}
	return syncDirectory(filepath.Dir(target))
}

func remove(root, relative string) error {
	target, err := resolve(root, relative, false)
	if err != nil {
		return err
	}
	if target == filepath.Clean(root) {
		return errors.New("allowed root cannot be deleted")
	}
	if err := os.Remove(target); err != nil {
		return errors.New("cannot delete non-empty or protected path")
	}
	return syncDirectory(filepath.Dir(target))
}

func syncDirectory(path string) error {
	directory, err := os.Open(path)
	if err != nil {
		return errors.New("cannot open parent directory")
	}
	defer func() { _ = directory.Close() }()
	if err := directory.Sync(); err != nil {
		return errors.New("cannot sync parent directory")
	}
	return nil
}
