package secrets

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const testToken = "123456789:ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghi_123"

func TestValidateTelegramToken(t *testing.T) {
	if err := ValidateTelegramToken(testToken); err != nil {
		t.Fatalf("valid token rejected: %v", err)
	}
	for _, token := range []string{"", "token", "123:short", testToken + "\nINJECTED=x"} {
		if ValidateTelegramToken(token) == nil {
			t.Fatalf("invalid token accepted: %q", token)
		}
	}
}

func TestReplaceEnvironmentValuePreservesOtherSecretsAndMode(t *testing.T) {
	path := filepath.Join(t.TempDir(), "secrets.env")
	if err := os.WriteFile(path, []byte("JWT_SECRET=keep\n"+EnvironmentKey+"=old\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := replaceEnvironmentValue(path, EnvironmentKey, testToken); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	if !strings.Contains(content, "JWT_SECRET=keep") || !strings.Contains(content, EnvironmentKey+"="+testToken) || strings.Contains(content, "=old") {
		t.Fatalf("unexpected secrets content: %q", content)
	}
	info, _ := os.Stat(path)
	if info.Mode().Perm() != 0o640 {
		t.Fatalf("mode changed: %o", info.Mode().Perm())
	}
}

func TestTelegramTokenReadsFileWithoutChangingEnvironment(t *testing.T) {
	t.Setenv(EnvironmentKey, "")
	path := filepath.Join(t.TempDir(), "secrets.env")
	if err := os.WriteFile(path, []byte(EnvironmentKey+"="+testToken+"\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	if token := TelegramToken(path); token != testToken {
		t.Fatalf("unexpected token: %q", token)
	}
}
