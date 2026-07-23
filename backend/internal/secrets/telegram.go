package secrets

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
)

const (
	EnvironmentKey = "MINI_UBUNTU_SERVER_TELEGRAM_BOT_TOKEN"
	DefaultPath    = "/etc/mini-ubuntu-server/secrets.env"
	maxTokenBytes  = 512
)

var tokenPattern = regexp.MustCompile(`^[0-9]{6,16}:[A-Za-z0-9_-]{30,256}$`)

type Writer interface {
	SetTelegramToken(ctx context.Context, token string) error
}

type SudoWriter struct {
	Executable string
}

func NewSudoWriter() (*SudoWriter, error) {
	executable, err := os.Executable()
	if err != nil {
		return nil, err
	}
	return &SudoWriter{Executable: executable}, nil
}

func ValidateTelegramToken(token string) error {
	if !tokenPattern.MatchString(token) {
		return errors.New("invalid Telegram Bot Token")
	}
	return nil
}

func (w SudoWriter) SetTelegramToken(ctx context.Context, token string) error {
	if err := ValidateTelegramToken(token); err != nil {
		return err
	}
	command := exec.CommandContext(ctx, "/usr/bin/sudo", "-n", w.Executable, "privileged-secret", "telegram-token")
	command.Stdin = strings.NewReader(token + "\n")
	if output, err := command.CombinedOutput(); err != nil {
		return fmt.Errorf("privileged secret update failed: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}

func RunPrivilegedTelegramToken(input io.Reader, path string) error {
	if os.Geteuid() != 0 {
		return errors.New("privileged-secret must run as root")
	}
	data, err := io.ReadAll(io.LimitReader(input, maxTokenBytes+1))
	if err != nil || len(data) > maxTokenBytes {
		return errors.New("invalid secret input")
	}
	token := strings.TrimSpace(string(data))
	if err := ValidateTelegramToken(token); err != nil {
		return err
	}
	return replaceEnvironmentValue(path, EnvironmentKey, token)
}

func TelegramToken(path string) string {
	file, err := os.Open(path)
	if err == nil {
		defer func() { _ = file.Close() }()
		scanner := bufio.NewScanner(io.LimitReader(file, 64*1024))
		for scanner.Scan() {
			key, value, found := strings.Cut(scanner.Text(), "=")
			if found && key == EnvironmentKey {
				return strings.TrimSpace(value)
			}
		}
	}
	return strings.TrimSpace(os.Getenv(EnvironmentKey))
}

func replaceEnvironmentValue(path, key, value string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	stat, statOK := info.Sys().(*syscall.Stat_t)
	if !info.Mode().IsRegular() || info.Mode().Perm()&0o037 != 0 || !statOK || (os.Geteuid() == 0 && stat.Uid != 0) {
		return errors.New("unsafe secrets file permissions")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	lines := strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
	updated := false
	for index, line := range lines {
		lineKey, _, found := strings.Cut(line, "=")
		if found && lineKey == key {
			lines[index] = key + "=" + value
			updated = true
		}
	}
	if !updated {
		lines = append(lines, key+"="+value)
	}
	content := []byte(strings.Join(lines, "\n") + "\n")
	directory := filepath.Dir(path)
	temporary, err := os.CreateTemp(directory, ".secrets-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	cleanup := true
	defer func() {
		_ = temporary.Close()
		if cleanup {
			_ = os.Remove(temporaryPath)
		}
	}()
	if err := temporary.Chmod(info.Mode().Perm()); err != nil {
		return err
	}
	if err := temporary.Chown(int(stat.Uid), int(stat.Gid)); err != nil {
		return err
	}
	if _, err := temporary.Write(content); err != nil {
		return err
	}
	if err := temporary.Sync(); err != nil {
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return err
	}
	cleanup = false
	directoryHandle, err := os.Open(directory)
	if err != nil {
		return err
	}
	defer func() { _ = directoryHandle.Close() }()
	return directoryHandle.Sync()
}
