package files

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveRejectsTraversalAndSymlinks(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "safe.txt"), []byte("safe"), 0600); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "secret.txt")
	if err := os.WriteFile(outside, []byte("secret"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "link.txt")); err != nil {
		t.Fatal(err)
	}
	if _, err := resolve(root, "safe.txt", false); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"../secret", "/etc/passwd", "link.txt"} {
		if _, err := resolve(root, path, false); err == nil {
			t.Fatalf("unsafe path accepted: %q", path)
		}
	}
}

func TestFileOperationsAreBoundedAndAtomic(t *testing.T) {
	root := t.TempDir()
	if err := mkdir(root, "configs"); err != nil {
		t.Fatal(err)
	}
	if err := write(root, "configs/app.yml", []byte("enabled: true\n")); err != nil {
		t.Fatal(err)
	}
	file, err := read(root, "configs/app.yml")
	if err != nil || file.Content != "enabled: true\n" {
		t.Fatalf("unexpected file: %#v err=%v", file, err)
	}
	entries, err := list(root, "configs")
	if err != nil || len(entries) != 1 || entries[0].Name != "app.yml" {
		t.Fatalf("unexpected entries: %#v err=%v", entries, err)
	}
	if err := write(root, "too-large", []byte(strings.Repeat("x", MaxContentBytes+1))); err == nil {
		t.Fatal("oversized content accepted")
	}
	if err := remove(root, "configs/app.yml"); err != nil {
		t.Fatal(err)
	}
}

func TestValidateRequestRejectsRootAndOperationAbuse(t *testing.T) {
	requests := []request{
		{Operation: "read", Root: -1, Path: "file"},
		{Operation: "exec", Root: 0, Path: "file"},
		{Operation: "delete", Root: 0, Path: ""},
		{Operation: "write", Root: 0, Path: "../file", Content: []byte("x")},
	}
	for _, value := range requests {
		if err := validateRequest(value, 1); err == nil {
			t.Fatalf("unsafe request accepted: %#v", value)
		}
	}
}
