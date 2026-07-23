package updater

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/user"
	"path/filepath"
	"strings"
)

type UninstallOptions struct {
	Yes           bool
	RemoveConfig  bool
	RemoveData    bool
	RemoveBackups bool
	RemoveUser    bool
	ConfigPath    string
	DataDir       string
	Input         io.Reader
	Output        io.Writer
}

func RunUninstall(ctx context.Context, options UninstallOptions) error {
	if os.Geteuid() != 0 {
		return errors.New("uninstall must run as root")
	}
	if options.Input == nil {
		options.Input = os.Stdin
	}
	if options.Output == nil {
		options.Output = os.Stdout
	}
	if options.ConfigPath == "" {
		options.ConfigPath = defaultConfigPath
	}
	if options.DataDir == "" {
		options.DataDir = dataDirectoryFromConfig(options.ConfigPath)
	}
	reader := bufio.NewReader(io.LimitReader(options.Input, 4096))
	removeApplication := options.Yes
	if !options.Yes {
		removeApplication = confirm(reader, options.Output, "Remove application", false)
		if !removeApplication {
			return nil
		}
		options.RemoveConfig = confirm(reader, options.Output, "Remove configuration", false)
		options.RemoveData = confirm(reader, options.Output, "Remove SQLite and metric history", false)
		options.RemoveBackups = confirm(reader, options.Output, "Remove backups", false)
		options.RemoveUser = confirm(reader, options.Output, "Remove system user mini-ubuntu-server", false)
	}
	if !removeApplication {
		return nil
	}
	_ = runCommand(ctx, "systemctl", "disable", "--now", defaultService)
	for _, path := range []string{
		"/etc/systemd/system/mini-ubuntu-server.service",
		"/etc/sudoers.d/mini-ubuntu-server",
		"/usr/local/bin/mini-ubuntu-server",
	} {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("remove %s: %w", path, err)
		}
	}
	if err := os.RemoveAll(defaultInstallDir); err != nil {
		return errors.New("remove application directory")
	}
	if options.RemoveConfig {
		if err := os.RemoveAll("/etc/mini-ubuntu-server"); err != nil {
			return errors.New("remove configuration")
		}
	}
	if options.RemoveData {
		for _, suffix := range []string{"", "-wal", "-shm"} {
			path := filepath.Join(options.DataDir, "mini-ubuntu-server.db"+suffix)
			if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
				return errors.New("remove SQLite data")
			}
		}
	}
	if options.RemoveBackups {
		if err := os.RemoveAll(filepath.Join(options.DataDir, "backups")); err != nil {
			return errors.New("remove backups")
		}
	}
	if options.RemoveUser {
		if _, lookupErr := user.Lookup("mini-ubuntu-server"); lookupErr == nil {
			if err := runCommand(ctx, "userdel", "mini-ubuntu-server"); err != nil {
				return errors.New("remove system user")
			}
		}
	}
	if err := runCommand(ctx, "systemctl", "daemon-reload"); err != nil {
		return errors.New("reload systemd")
	}
	_, _ = fmt.Fprintln(options.Output, "Mini Ubuntu Server Panel removed. Unselected data was preserved.")
	return nil
}

func confirm(reader *bufio.Reader, output io.Writer, question string, defaultYes bool) bool {
	suffix := " [y/N] "
	if defaultYes {
		suffix = " [Y/n] "
	}
	_, _ = fmt.Fprint(output, question+"?"+suffix)
	answer, _ := reader.ReadString('\n')
	answer = strings.ToLower(strings.TrimSpace(answer))
	if answer == "" {
		return defaultYes
	}
	return answer == "y" || answer == "yes"
}
