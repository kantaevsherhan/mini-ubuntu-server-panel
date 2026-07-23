package systemusers

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

const maxRequestBytes = 32 * 1024

var (
	usernamePattern = regexp.MustCompile(`^[a-z_][a-z0-9_-]{2,31}$`)
	groupPattern    = regexp.MustCompile(`^[a-z_][a-z0-9_-]{0,31}$`)
	allowedShells   = map[string]bool{
		"/bin/bash": true, "/bin/sh": true, "/usr/bin/bash": true,
		"/usr/sbin/nologin": true, "/bin/false": true,
	}
)

type User struct {
	Username string   `json:"username"`
	UID      int      `json:"uid"`
	GID      int      `json:"gid"`
	Home     string   `json:"home"`
	Shell    string   `json:"shell"`
	Groups   []string `json:"groups"`
	HasSudo  bool     `json:"has_sudo"`
	HasSSH   bool     `json:"has_ssh_keys"`
}

type CreateRequest struct {
	Username      string   `json:"username"`
	HomeDirectory string   `json:"home_directory"`
	Shell         string   `json:"shell"`
	Groups        []string `json:"groups"`
	AllowSudo     bool     `json:"allow_sudo"`
	CreateHome    bool     `json:"create_home"`
	AllowSSH      bool     `json:"allow_ssh"`
	SSHPublicKey  string   `json:"ssh_public_key"`
}

type DeleteRequest struct {
	Username   string `json:"username"`
	RemoveHome bool   `json:"remove_home"`
}

type privilegedRequest struct {
	Action string         `json:"action"`
	Create *CreateRequest `json:"create,omitempty"`
	Delete *DeleteRequest `json:"delete,omitempty"`
}

type Client interface {
	Exists(username string) (bool, error)
	Create(ctx context.Context, request CreateRequest) error
	Delete(ctx context.Context, request DeleteRequest) error
}

type SudoClient struct {
	Executable string
}

func NewSudoClient() (*SudoClient, error) {
	executable, err := os.Executable()
	if err != nil {
		return nil, err
	}
	return &SudoClient{Executable: executable}, nil
}

func (c SudoClient) Exists(username string) (bool, error) {
	if !usernamePattern.MatchString(username) {
		return false, ErrInvalidRequest
	}
	_, err := user.Lookup(username)
	if err == nil {
		return true, nil
	}
	if _, ok := err.(user.UnknownUserError); ok {
		return false, nil
	}
	return false, err
}

func (c SudoClient) Create(ctx context.Context, request CreateRequest) error {
	return c.call(ctx, privilegedRequest{Action: "create", Create: &request})
}

func (c SudoClient) Delete(ctx context.Context, request DeleteRequest) error {
	return c.call(ctx, privilegedRequest{Action: "delete", Delete: &request})
}

func (c SudoClient) call(ctx context.Context, request privilegedRequest) error {
	payload, err := json.Marshal(request)
	if err != nil {
		return err
	}
	command := exec.CommandContext(ctx, "/usr/bin/sudo", "-n", c.Executable, "privileged-user")
	command.Stdin = strings.NewReader(string(payload))
	output, err := command.CombinedOutput()
	if err != nil {
		return fmt.Errorf("privileged user operation failed: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}

var ErrInvalidRequest = errors.New("invalid system user request")

func RunPrivileged(input io.Reader) error {
	if os.Geteuid() != 0 {
		return errors.New("privileged-user must run as root")
	}
	decoder := json.NewDecoder(io.LimitReader(input, maxRequestBytes))
	decoder.DisallowUnknownFields()
	var request privilegedRequest
	if err := decoder.Decode(&request); err != nil {
		return ErrInvalidRequest
	}
	if err := ensureEOF(decoder); err != nil {
		return err
	}
	switch {
	case request.Action == "create" && request.Create != nil && request.Delete == nil:
		return create(*request.Create)
	case request.Action == "delete" && request.Delete != nil && request.Create == nil:
		return remove(*request.Delete)
	default:
		return ErrInvalidRequest
	}
}

func ensureEOF(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return ErrInvalidRequest
	}
	return nil
}

func create(request CreateRequest) error {
	if err := validateCreate(request); err != nil {
		return err
	}
	if _, err := os.Lstat(request.HomeDirectory); err == nil || !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("home directory already exists or cannot be inspected")
	}
	if _, err := user.Lookup(request.Username); err == nil {
		return fmt.Errorf("system user already exists")
	} else if _, ok := err.(user.UnknownUserError); !ok {
		return err
	}

	groups := append([]string(nil), request.Groups...)
	if request.AllowSudo && !contains(groups, "sudo") {
		groups = append(groups, "sudo")
	}
	args := []string{"--shell", request.Shell, "--home-dir", request.HomeDirectory}
	if request.CreateHome {
		args = append(args, "--create-home")
	} else {
		args = append(args, "--no-create-home")
	}
	if len(groups) > 0 {
		args = append(args, "--groups", strings.Join(groups, ","))
	}
	args = append(args, request.Username)
	if output, err := exec.Command("/usr/sbin/useradd", args...).CombinedOutput(); err != nil {
		return fmt.Errorf("useradd failed: %w: %s", err, strings.TrimSpace(string(output)))
	}
	if request.AllowSSH && request.SSHPublicKey != "" {
		if err := installSSHKey(request); err != nil {
			_ = remove(DeleteRequest{Username: request.Username, RemoveHome: request.CreateHome})
			return err
		}
	}
	return nil
}

func remove(request DeleteRequest) error {
	if !usernamePattern.MatchString(request.Username) || request.Username == "root" {
		return ErrInvalidRequest
	}
	args := make([]string, 0, 2)
	if request.RemoveHome {
		args = append(args, "--remove")
	}
	args = append(args, request.Username)
	output, err := exec.Command("/usr/sbin/userdel", args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("userdel failed: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}

func validateCreate(request CreateRequest) error {
	if !usernamePattern.MatchString(request.Username) || request.Username == "root" || !allowedShells[request.Shell] {
		return ErrInvalidRequest
	}
	cleanHome := filepath.Clean(request.HomeDirectory)
	if !filepath.IsAbs(cleanHome) || cleanHome != request.HomeDirectory || cleanHome == "/" || !strings.HasPrefix(cleanHome, "/home/") {
		return ErrInvalidRequest
	}
	for _, group := range request.Groups {
		if !groupPattern.MatchString(group) || group == "root" {
			return ErrInvalidRequest
		}
		if _, err := user.LookupGroup(group); err != nil {
			return ErrInvalidRequest
		}
	}
	key := strings.TrimSpace(request.SSHPublicKey)
	if len(key) > 16*1024 || strings.ContainsAny(key, "\r\n") {
		return ErrInvalidRequest
	}
	if key != "" && !validSSHKey(key) {
		return ErrInvalidRequest
	}
	if request.AllowSSH && key == "" {
		return ErrInvalidRequest
	}
	return nil
}

func validSSHKey(key string) bool {
	fields := strings.Fields(key)
	if len(fields) < 2 {
		return false
	}
	switch fields[0] {
	case "ssh-ed25519", "ssh-rsa", "ecdsa-sha2-nistp256", "ecdsa-sha2-nistp384", "ecdsa-sha2-nistp521":
		return len(fields[1]) >= 32
	default:
		return false
	}
}

func installSSHKey(request CreateRequest) error {
	account, err := user.Lookup(request.Username)
	if err != nil {
		return err
	}
	uid, err := strconv.Atoi(account.Uid)
	if err != nil {
		return err
	}
	gid, err := strconv.Atoi(account.Gid)
	if err != nil {
		return err
	}
	sshDirectory := filepath.Join(request.HomeDirectory, ".ssh")
	if err := os.MkdirAll(sshDirectory, 0o700); err != nil {
		return err
	}
	if err := os.Chown(sshDirectory, uid, gid); err != nil {
		return err
	}
	keyPath := filepath.Join(sshDirectory, "authorized_keys")
	if err := os.WriteFile(keyPath, []byte(strings.TrimSpace(request.SSHPublicKey)+"\n"), 0o600); err != nil {
		return err
	}
	return os.Chown(keyPath, uid, gid)
}

func contains(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}

func List() ([]User, error) {
	file, err := os.Open("/etc/passwd")
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()
	var result []User
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		parts := strings.Split(scanner.Text(), ":")
		if len(parts) != 7 {
			continue
		}
		uid, _ := strconv.Atoi(parts[2])
		gid, _ := strconv.Atoi(parts[3])
		groups, _ := userGroups(parts[0])
		result = append(result, User{
			Username: parts[0], UID: uid, GID: gid, Home: parts[5], Shell: parts[6], Groups: groups,
			HasSudo: contains(groups, "sudo"), HasSSH: hasSSHKeys(parts[5]),
		})
	}
	return result, scanner.Err()
}

func userGroups(username string) ([]string, error) {
	account, err := user.Lookup(username)
	if err != nil {
		return nil, err
	}
	ids, err := account.GroupIds()
	if err != nil {
		return nil, err
	}
	groups := make([]string, 0, len(ids))
	for _, id := range ids {
		group, err := user.LookupGroupId(id)
		if err == nil {
			groups = append(groups, group.Name)
		}
	}
	return groups, nil
}

func hasSSHKeys(home string) bool {
	info, err := os.Stat(filepath.Join(home, ".ssh", "authorized_keys"))
	return err == nil && info.Mode().IsRegular() && info.Size() > 0
}
