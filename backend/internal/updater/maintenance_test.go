package updater

import (
	"archive/tar"
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/kantaevsherhan/mini-ubuntu-server-panel/backend/internal/database"
)

type updateMigrationProbe struct {
	ID int64 `gorm:"primaryKey"`
}

func releaseArchive(t *testing.T, name string, content []byte, kind byte) []byte {
	t.Helper()
	var result bytes.Buffer
	gzipWriter := gzip.NewWriter(&result)
	tarWriter := tar.NewWriter(gzipWriter)
	if err := tarWriter.WriteHeader(&tar.Header{Name: name, Mode: 0755, Size: int64(len(content)), Typeflag: kind}); err != nil {
		t.Fatal(err)
	}
	if kind == tar.TypeReg {
		if _, err := tarWriter.Write(content); err != nil {
			t.Fatal(err)
		}
	}
	if err := tarWriter.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gzipWriter.Close(); err != nil {
		t.Fatal(err)
	}
	return result.Bytes()
}

func TestChecksumAndArchiveValidation(t *testing.T) {
	archive := releaseArchive(t, "mini-ubuntu-server", []byte("binary"), tar.TypeReg)
	digest := sha256.Sum256(archive)
	checksums := []byte(fmt.Sprintf("%x  mini-ubuntu-server-linux-amd64.tar.gz\n", digest))
	if err := verifyChecksum("mini-ubuntu-server-linux-amd64.tar.gz", archive, checksums); err != nil {
		t.Fatal(err)
	}
	if checksums[0] == '0' {
		checksums[0] = '1'
	} else {
		checksums[0] = '0'
	}
	if err := verifyChecksum("mini-ubuntu-server-linux-amd64.tar.gz", archive, checksums); err == nil {
		t.Fatal("modified checksum accepted")
	}
	if binary, err := extractBinary(archive); err != nil || string(binary) != "binary" {
		t.Fatalf("valid binary rejected: %q %v", binary, err)
	}
	if _, err := extractBinary(releaseArchive(t, "mini-ubuntu-server", nil, tar.TypeSymlink)); err == nil {
		t.Fatal("symlink binary accepted")
	}
}

func TestPerformUpdateSuccessAndRollback(t *testing.T) {
	for _, failHealth := range []bool{false, true} {
		t.Run(fmt.Sprintf("fail_health_%t", failHealth), func(t *testing.T) {
			root := t.TempDir()
			installDir := filepath.Join(root, "opt")
			dataDir := filepath.Join(root, "data")
			if err := os.MkdirAll(filepath.Join(installDir, "bin"), 0750); err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(dataDir, 0750); err != nil {
				t.Fatal(err)
			}
			binaryPath := filepath.Join(installDir, "bin", "mini-ubuntu-server")
			databasePath := filepath.Join(dataDir, "mini-ubuntu-server.db")
			if err := os.WriteFile(binaryPath, []byte("old-binary"), 0755); err != nil {
				t.Fatal(err)
			}
			panelDB, err := database.Open(databasePath)
			if err != nil {
				t.Fatal(err)
			}
			sqlDB, err := panelDB.DB()
			if err != nil || sqlDB.Close() != nil {
				t.Fatal("failed to close initial database")
			}
			archiveName := "mini-ubuntu-server-linux-" + runtime.GOARCH + ".tar.gz"
			archive := releaseArchive(t, "mini-ubuntu-server", []byte("new-binary"), tar.TypeReg)
			digest := sha256.Sum256(archive)
			checksums := []byte(fmt.Sprintf("%x  %s\n", digest, archiveName))
			commands := []string{}
			dependencies := updateDependencies{
				download: func(_ context.Context, address string, _ int64) ([]byte, error) {
					if strings.HasSuffix(address, "checksums.txt") {
						return checksums, nil
					}
					return archive, nil
				},
				run: func(_ context.Context, name string, arguments ...string) error {
					commands = append(commands, name+" "+strings.Join(arguments, " "))
					return nil
				},
				waitHealth: func(context.Context, string) error {
					migratedDB, openErr := database.Open(databasePath)
					if openErr != nil {
						return openErr
					}
					if migrationErr := migratedDB.Migrator().CreateTable(&updateMigrationProbe{}); migrationErr != nil {
						return migrationErr
					}
					migratedSQL, sqlErr := migratedDB.DB()
					if sqlErr != nil {
						return sqlErr
					}
					if closeErr := migratedSQL.Close(); closeErr != nil {
						return closeErr
					}
					if failHealth {
						return errors.New("health failed")
					}
					return nil
				},
				now: func() time.Time { return time.Unix(1_000, 0) }, checker: NewHTTPChecker(),
			}
			options := normalizeUpdateOptions(UpdateOptions{
				Version: "v1.2.3", CurrentVersion: "v1.2.2", InstallDir: installDir,
				DataDir: dataDir, ConfigPath: filepath.Join(root, "config.yml"), Service: defaultService,
				LockPath: filepath.Join(root, "update.lock"),
			})
			err = performUpdate(context.Background(), options, dependencies)
			binary, _ := os.ReadFile(binaryPath)
			verifiedDB, openErr := database.Open(databasePath)
			if openErr != nil {
				t.Fatal(openErr)
			}
			migrationPresent := verifiedDB.Migrator().HasTable(&updateMigrationProbe{})
			verifiedSQL, _ := verifiedDB.DB()
			_ = verifiedSQL.Close()
			if failHealth {
				if err == nil || string(binary) != "old-binary" || migrationPresent {
					t.Fatalf("rollback failed: err=%v binary=%q migration_present=%t", err, binary, migrationPresent)
				}
			} else if err != nil || string(binary) != "new-binary" || !migrationPresent {
				t.Fatalf("update failed: err=%v binary=%q migration_present=%t", err, binary, migrationPresent)
			}
			if len(commands) < 2 {
				t.Fatalf("systemd lifecycle not executed: %#v", commands)
			}
		})
	}
}

func TestHealthAddressReadsConfiguredPort(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yml")
	if err := os.WriteFile(path, []byte("listen: 0.0.0.0:9123\n"), 0600); err != nil {
		t.Fatal(err)
	}
	address, err := healthAddress(path)
	if err != nil || address != "http://127.0.0.1:9123/api/v1/health" {
		t.Fatalf("unexpected health address: %q %v", address, err)
	}
}

func TestConfirmDefaultsToNo(t *testing.T) {
	if confirm(bufio.NewReader(strings.NewReader("\n")), &bytes.Buffer{}, "Remove data", false) {
		t.Fatal("destructive confirmation defaulted to yes")
	}
}

func multiFileArchive(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var result bytes.Buffer
	gzipWriter := gzip.NewWriter(&result)
	tarWriter := tar.NewWriter(gzipWriter)
	for name, content := range files {
		if err := tarWriter.WriteHeader(&tar.Header{Name: name, Mode: 0644, Size: int64(len(content)), Typeflag: tar.TypeReg}); err != nil {
			t.Fatal(err)
		}
		if _, err := tarWriter.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := tarWriter.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gzipWriter.Close(); err != nil {
		t.Fatal(err)
	}
	return result.Bytes()
}

func TestPerformUpdateInstallsSudoersAndUnitWithRollback(t *testing.T) {
	for _, failHealth := range []bool{false, true} {
		t.Run(fmt.Sprintf("fail_health_%t", failHealth), func(t *testing.T) {
			root := t.TempDir()
			installDir, dataDir := filepath.Join(root, "opt"), filepath.Join(root, "data")
			for _, dir := range []string{filepath.Join(installDir, "bin"), dataDir} {
				if err := os.MkdirAll(dir, 0750); err != nil {
					t.Fatal(err)
				}
			}
			binaryPath := filepath.Join(installDir, "bin", "mini-ubuntu-server")
			sudoersPath, unitPath := filepath.Join(root, "sudoers"), filepath.Join(root, "panel.service")
			for path, content := range map[string]string{binaryPath: "old-binary", sudoersPath: "old-sudoers", unitPath: "old-unit"} {
				if err := os.WriteFile(path, []byte(content), 0644); err != nil {
					t.Fatal(err)
				}
			}
			// Stale backups beyond the retention limit must be pruned after success.
			for index := range 7 {
				if err := os.MkdirAll(filepath.Join(dataDir, "backups", fmt.Sprintf("update-2000010%dT000000Z", index)), 0750); err != nil {
					t.Fatal(err)
				}
			}
			archiveName := "mini-ubuntu-server-linux-" + runtime.GOARCH + ".tar.gz"
			archive := multiFileArchive(t, map[string]string{"mini-ubuntu-server": "new-binary", "mini-ubuntu-server.sudoers": "new-sudoers", "mini-ubuntu-server.service": "new-unit"})
			digest := sha256.Sum256(archive)
			commands := []string{}
			dependencies := updateDependencies{
				download: func(_ context.Context, address string, _ int64) ([]byte, error) {
					if strings.HasSuffix(address, "checksums.txt") {
						return []byte(fmt.Sprintf("%x  %s\n", digest, archiveName)), nil
					}
					return archive, nil
				},
				run: func(_ context.Context, name string, arguments ...string) error {
					commands = append(commands, name+" "+strings.Join(arguments, " "))
					return nil
				},
				waitHealth: func(context.Context, string) error {
					if failHealth {
						return errors.New("health failed")
					}
					return nil
				},
				now: func() time.Time { return time.Unix(2_000_000_000, 0) }, checker: NewHTTPChecker(),
			}
			options := normalizeUpdateOptions(UpdateOptions{
				Version: "v1.2.3", InstallDir: installDir, DataDir: dataDir, ConfigPath: filepath.Join(root, "config.yml"),
				LockPath: filepath.Join(root, "update.lock"), SudoersPath: sudoersPath, UnitPath: unitPath,
			})
			err := performUpdate(context.Background(), options, dependencies)
			sudoers, _ := os.ReadFile(sudoersPath)
			unit, _ := os.ReadFile(unitPath)
			if !strings.Contains(strings.Join(commands, "\n"), "visudo -cf") {
				t.Fatalf("sudoers not validated: %#v", commands)
			}
			if failHealth {
				if err == nil || string(sudoers) != "old-sudoers" || string(unit) != "old-unit" {
					t.Fatalf("rollback failed: err=%v sudoers=%q unit=%q", err, sudoers, unit)
				}
				return
			}
			if err != nil || string(sudoers) != "new-sudoers" || string(unit) != "new-unit" {
				t.Fatalf("update failed: err=%v sudoers=%q unit=%q", err, sudoers, unit)
			}
			entries, _ := os.ReadDir(filepath.Join(dataDir, "backups"))
			if len(entries) != keptUpdateBackups {
				t.Fatalf("expected %d backups after pruning, got %d", keptUpdateBackups, len(entries))
			}
		})
	}
}
