package updater

import (
	"archive/tar"
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"gopkg.in/yaml.v3"
)

const (
	defaultInstallDir = "/opt/mini-ubuntu-server"
	defaultDataDir    = "/var/lib/mini-ubuntu-server"
	defaultConfigPath = "/etc/mini-ubuntu-server/config.yml"
	defaultService    = "mini-ubuntu-server.service"
	maximumDownload   = 150 * 1024 * 1024
	maximumBinary     = 100 * 1024 * 1024
)

type UpdateOptions struct {
	Version        string
	CurrentVersion string
	InstallDir     string
	DataDir        string
	ConfigPath     string
	Service        string
	LockPath       string
}

type updateDependencies struct {
	download   func(context.Context, string, int64) ([]byte, error)
	run        func(context.Context, string, ...string) error
	waitHealth func(context.Context, string) error
	now        func() time.Time
	checker    Checker
}

func RunUpdate(ctx context.Context, options UpdateOptions) error {
	if os.Geteuid() != 0 {
		return errors.New("update must run as root")
	}
	return performUpdate(ctx, normalizeUpdateOptions(options), updateDependencies{
		download: downloadBounded, run: runCommand, waitHealth: waitForHealth,
		now: time.Now, checker: NewHTTPChecker(),
	})
}

func normalizeUpdateOptions(options UpdateOptions) UpdateOptions {
	if options.Version == "" {
		options.Version = "latest"
	}
	if options.InstallDir == "" {
		options.InstallDir = defaultInstallDir
	}
	if options.ConfigPath == "" {
		options.ConfigPath = defaultConfigPath
	}
	if options.DataDir == "" {
		options.DataDir = dataDirectoryFromConfig(options.ConfigPath)
	}
	if options.Service == "" {
		options.Service = defaultService
	}
	if options.LockPath == "" {
		options.LockPath = "/run/mini-ubuntu-server-update.lock"
	}
	return options
}

func dataDirectoryFromConfig(configPath string) string {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return defaultDataDir
	}
	var value struct {
		DataDir string `yaml:"data_dir"`
	}
	if yaml.Unmarshal(data, &value) != nil || !filepath.IsAbs(value.DataDir) || filepath.Clean(value.DataDir) == string(filepath.Separator) {
		return defaultDataDir
	}
	return filepath.Clean(value.DataDir)
}

func performUpdate(ctx context.Context, options UpdateOptions, dependencies updateDependencies) (resultErr error) {
	if runtime.GOOS != "linux" || (runtime.GOARCH != "amd64" && runtime.GOARCH != "arm64") {
		return errors.New("update supports Linux amd64 and arm64 only")
	}
	lock, err := acquireUpdateLock(options.LockPath)
	if err != nil {
		return err
	}
	defer func() { _ = lock.Close() }()

	version := options.Version
	if version == "latest" {
		status, checkErr := dependencies.checker.Check(ctx, options.CurrentVersion)
		if checkErr != nil {
			return checkErr
		}
		version = status.Latest
	}
	if !versionPattern.MatchString(version) {
		return errors.New("invalid release version")
	}
	archiveName := "mini-ubuntu-server-linux-" + runtime.GOARCH + ".tar.gz"
	baseURL := "https://github.com/kantaevsherhan/mini-ubuntu-server-panel/releases/download/" + version + "/"
	archive, err := dependencies.download(ctx, baseURL+archiveName, maximumDownload)
	if err != nil {
		return fmt.Errorf("download release archive: %w", err)
	}
	checksums, err := dependencies.download(ctx, baseURL+"checksums.txt", 1024*1024)
	if err != nil {
		return fmt.Errorf("download checksums: %w", err)
	}
	if err := verifyChecksum(archiveName, archive, checksums); err != nil {
		return err
	}
	binary, err := extractBinary(archive)
	if err != nil {
		return err
	}

	backupDir := filepath.Join(options.DataDir, "backups", "update-"+dependencies.now().UTC().Format("20060102T150405Z"))
	if err := os.MkdirAll(backupDir, 0750); err != nil {
		return errors.New("create update backup directory")
	}
	binaryPath := filepath.Join(options.InstallDir, "bin", "mini-ubuntu-server")
	backupBinary := filepath.Join(backupDir, "mini-ubuntu-server")
	if err := dependencies.run(ctx, "systemctl", "stop", options.Service); err != nil {
		return errors.New("stop service before update")
	}
	backupReady := false
	rollbackNeeded := true
	defer func() {
		if resultErr == nil || !rollbackNeeded {
			return
		}
		_ = dependencies.run(context.Background(), "systemctl", "stop", options.Service)
		if backupReady {
			_ = copyFileAtomic(backupBinary, binaryPath, 0755)
			_ = restoreDatabaseBackup(options.DataDir, backupDir)
		}
		_ = dependencies.run(context.Background(), "systemctl", "start", options.Service)
	}()

	if err := copyFile(binaryPath, backupBinary, 0750); err != nil {
		return errors.New("backup current binary")
	}
	if err := backupDatabase(options.DataDir, backupDir); err != nil {
		return errors.New("backup SQLite database")
	}
	backupReady = true
	if err := writeFileAtomic(binaryPath, binary, 0755); err != nil {
		return errors.New("install new binary")
	}
	if err := dependencies.run(ctx, "systemctl", "start", options.Service); err != nil {
		return errors.New("start updated service")
	}
	if err := dependencies.waitHealth(ctx, options.ConfigPath); err != nil {
		return errors.New("updated service failed health check")
	}
	rollbackNeeded = false
	return nil
}

func acquireUpdateLock(path string) (*os.File, error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, errors.New("open update lock")
	}
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = file.Close()
		return nil, errors.New("another update is already running")
	}
	return file, nil
}

func downloadBounded(ctx context.Context, address string, maximum int64) ([]byte, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return nil, errors.New("invalid download request")
	}
	request.Header.Set("User-Agent", "mini-ubuntu-server")
	client := &http.Client{Timeout: 2 * time.Minute}
	response, err := client.Do(request)
	if err != nil {
		return nil, errors.New("download unavailable")
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		return nil, errors.New("download returned an error")
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, maximum+1))
	if err != nil || int64(len(data)) > maximum {
		return nil, errors.New("download exceeds size limit")
	}
	return data, nil
}

func verifyChecksum(filename string, data, checksums []byte) error {
	wanted := ""
	scanner := bufio.NewScanner(bytes.NewReader(checksums))
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) == 2 && strings.TrimPrefix(fields[1], "*") == filename {
			wanted = fields[0]
			break
		}
	}
	decoded, err := hex.DecodeString(wanted)
	digest := sha256.Sum256(data)
	if err != nil || len(decoded) != sha256.Size || !bytes.Equal(decoded, digest[:]) {
		return errors.New("release checksum verification failed")
	}
	return nil
}

func extractBinary(archive []byte) ([]byte, error) {
	gzipReader, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		return nil, errors.New("release archive is invalid")
	}
	defer func() { _ = gzipReader.Close() }()
	reader := tar.NewReader(gzipReader)
	for {
		header, nextErr := reader.Next()
		if errors.Is(nextErr, io.EOF) {
			break
		}
		if nextErr != nil {
			return nil, errors.New("release archive is invalid")
		}
		if header.Name != "mini-ubuntu-server" {
			continue
		}
		if header.Typeflag != tar.TypeReg || header.Size <= 0 || header.Size > maximumBinary {
			return nil, errors.New("release binary is invalid")
		}
		binary, readErr := io.ReadAll(io.LimitReader(reader, maximumBinary+1))
		if readErr != nil || int64(len(binary)) != header.Size {
			return nil, errors.New("release binary is invalid")
		}
		return binary, nil
	}
	return nil, errors.New("release binary is missing")
}

func backupDatabase(dataDir, backupDir string) error {
	for _, suffix := range []string{"", "-wal", "-shm"} {
		source := filepath.Join(dataDir, "mini-ubuntu-server.db"+suffix)
		if _, err := os.Stat(source); errors.Is(err, os.ErrNotExist) {
			continue
		} else if err != nil {
			return err
		}
		if err := copyFile(source, filepath.Join(backupDir, filepath.Base(source)), 0640); err != nil {
			return err
		}
	}
	return nil
}

func restoreDatabaseBackup(dataDir, backupDir string) error {
	for _, suffix := range []string{"", "-wal", "-shm"} {
		destination := filepath.Join(dataDir, "mini-ubuntu-server.db"+suffix)
		if err := os.Remove(destination); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		source := filepath.Join(backupDir, filepath.Base(destination))
		if _, err := os.Stat(source); errors.Is(err, os.ErrNotExist) {
			continue
		} else if err != nil {
			return err
		}
		if err := copyFileAtomic(source, destination, 0640); err != nil {
			return err
		}
	}
	return nil
}

func copyFile(source, destination string, mode os.FileMode) error {
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer func() { _ = input.Close() }()
	output, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	remove := true
	defer func() {
		_ = output.Close()
		if remove {
			_ = os.Remove(destination)
		}
	}()
	if _, err := io.Copy(output, input); err != nil {
		return err
	}
	if err := output.Sync(); err != nil {
		return err
	}
	if err := output.Close(); err != nil {
		return err
	}
	if err := copyOwnership(source, destination); err != nil {
		return err
	}
	remove = false
	return nil
}

func copyFileAtomic(source, destination string, mode os.FileMode) error {
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer func() { _ = input.Close() }()
	temporary, err := os.CreateTemp(filepath.Dir(destination), ".mini-ubuntu-server-copy-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer func() {
		_ = temporary.Close()
		_ = os.Remove(temporaryPath)
	}()
	if _, err := io.Copy(temporary, input); err != nil {
		return err
	}
	if err := temporary.Sync(); err != nil {
		return err
	}
	if err := temporary.Chmod(mode); err != nil {
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := copyOwnership(source, temporaryPath); err != nil {
		return err
	}
	return os.Rename(temporaryPath, destination)
}

func copyOwnership(source, destination string) error {
	info, err := os.Stat(source)
	if err != nil {
		return err
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return errors.New("file ownership is unavailable")
	}
	return os.Chown(destination, int(stat.Uid), int(stat.Gid))
}

func writeFileAtomic(destination string, data []byte, mode os.FileMode) error {
	temporary, err := os.CreateTemp(filepath.Dir(destination), ".mini-ubuntu-server-update-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer func() { _ = os.Remove(temporaryPath) }()
	if _, err := temporary.Write(data); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Chmod(mode); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(temporaryPath, destination)
}

func runCommand(ctx context.Context, name string, arguments ...string) error {
	command := exec.CommandContext(ctx, name, arguments...)
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	return command.Run()
}

func waitForHealth(ctx context.Context, configPath string) error {
	address, err := healthAddress(configPath)
	if err != nil {
		return err
	}
	client := &http.Client{Timeout: 2 * time.Second}
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		request, requestErr := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
		if requestErr == nil {
			response, responseErr := client.Do(request)
			if responseErr == nil {
				_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
				_ = response.Body.Close()
				if response.StatusCode == http.StatusOK {
					return nil
				}
			}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Second):
		}
	}
	return errors.New("health check timed out")
}

func healthAddress(configPath string) (string, error) {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return "", err
	}
	var value struct {
		Listen string `yaml:"listen"`
	}
	if yaml.Unmarshal(data, &value) != nil || value.Listen == "" {
		return "", errors.New("invalid listen configuration")
	}
	_, port, err := net.SplitHostPort(value.Listen)
	if err != nil {
		return "", errors.New("invalid listen configuration")
	}
	portNumber, err := strconv.Atoi(port)
	if err != nil || portNumber < 1 || portNumber > 65535 {
		return "", errors.New("invalid listen port")
	}
	return "http://127.0.0.1:" + port + "/api/v1/health", nil
}
