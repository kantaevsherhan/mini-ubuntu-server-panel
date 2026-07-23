package config

import "testing"

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
