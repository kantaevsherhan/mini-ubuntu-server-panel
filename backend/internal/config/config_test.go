package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNormalizeAllowedDirectoriesRejectsUnsafeRoots(t *testing.T) {
	if _, err := NormalizeAllowedDirectories([]string{"/var/lib/mini-ubuntu-server", "/var/log/mini-ubuntu-server"}); err != nil {
		t.Fatal(err)
	}
	for _, values := range [][]string{{"relative"}, {"/"}, {}} {
		if _, err := NormalizeAllowedDirectories(values); err == nil {
			t.Fatalf("unsafe roots accepted: %#v", values)
		}
	}
}

func TestLoadGeneratesAndReusesJWTSecret(t *testing.T) {
	t.Setenv("MINI_UBUNTU_SERVER_JWT_SECRET", "")
	root := t.TempDir()
	path := filepath.Join(root, "config.yml")
	content := "listen: \":8080\"\ndata_dir: " + filepath.ToSlash(filepath.Join(root, "data")) + "\nlog_dir: " + filepath.ToSlash(filepath.Join(root, "logs")) + "\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	first, err := Load(path)
	if err != nil || len(first.JWTSecret) < 32 {
		t.Fatalf("secret not generated: %v %q", err, first.JWTSecret)
	}
	stored, err := os.ReadFile(filepath.Join(first.DataDir, "jwt.key"))
	if err != nil || string(stored) != first.JWTSecret {
		t.Fatalf("secret not persisted: %v", err)
	}
	second, err := Load(path)
	if err != nil || second.JWTSecret != first.JWTSecret {
		t.Fatalf("secret not reused: %v", err)
	}
	// An explicit environment secret still wins over the stored one.
	t.Setenv("MINI_UBUNTU_SERVER_JWT_SECRET", "environment-secret-that-is-long-enough")
	third, err := Load(path)
	if err != nil || third.JWTSecret != "environment-secret-that-is-long-enough" {
		t.Fatalf("environment secret ignored: %v %q", err, third.JWTSecret)
	}
}
