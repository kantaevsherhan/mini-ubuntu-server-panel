package totp

import (
	"encoding/base32"
	"strings"
	"testing"
	"time"
)

// RFC 6238 appendix B vectors for SHA-1 (secret "12345678901234567890"), truncated to 6 digits.
func TestCodeMatchesRFC6238Vectors(t *testing.T) {
	secret := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString([]byte("12345678901234567890"))
	vectors := map[int64]string{59: "287082", 1111111109: "081804", 1111111111: "050471", 1234567890: "005924", 2000000000: "279037"}
	for unix, want := range vectors {
		got, err := Code(secret, Step(time.Unix(unix, 0)))
		if err != nil || got != want {
			t.Errorf("t=%d: got %q, want %q (err=%v)", unix, got, want, err)
		}
	}
}

func TestVerifyWindowAndReplay(t *testing.T) {
	secret, err := NewSecret()
	if err != nil {
		t.Fatal(err)
	}
	now := time.Unix(1_700_000_000, 0)
	previous, _ := Code(secret, Step(now)-1)
	step, ok := Verify(secret, previous, now, 0)
	if !ok || step != Step(now)-1 {
		t.Fatalf("previous-step code rejected: ok=%v step=%d", ok, step)
	}
	if _, ok := Verify(secret, previous, now, step); ok {
		t.Fatal("replayed code accepted")
	}
	stale, _ := Code(secret, Step(now)-3)
	if _, ok := Verify(secret, stale, now, 0); ok {
		t.Fatal("code outside the skew window accepted")
	}
	if _, ok := Verify(secret, "12345", now, 0); ok {
		t.Fatal("short code accepted")
	}
}

func TestSealerRoundTripAndTamper(t *testing.T) {
	sealer, err := NewSealer("test-secret-that-is-long-enough")
	if err != nil {
		t.Fatal(err)
	}
	sealed, err := sealer.Seal("JBSWY3DPEHPK3PXP")
	if err != nil || strings.Contains(sealed, "JBSWY3DP") {
		t.Fatalf("seal failed or leaked plaintext: %q %v", sealed, err)
	}
	plain, err := sealer.Open(sealed)
	if err != nil || plain != "JBSWY3DPEHPK3PXP" {
		t.Fatalf("open failed: %q %v", plain, err)
	}
	other, _ := NewSealer("another-secret-that-is-long-enough")
	if _, err := other.Open(sealed); err == nil {
		t.Fatal("value opened with a different key")
	}
}

func TestRecoveryCodes(t *testing.T) {
	codes, err := NewRecoveryCodes()
	if err != nil || len(codes) != RecoveryCodeCount {
		t.Fatalf("codes=%v err=%v", codes, err)
	}
	seen := map[string]bool{}
	for _, code := range codes {
		if len(code) != 11 || code[5] != '-' || seen[code] {
			t.Fatalf("bad or duplicate code %q", code)
		}
		seen[code] = true
	}
	if HashRecoveryCode(strings.ToUpper(strings.ReplaceAll(codes[0], "-", ""))) != HashRecoveryCode(codes[0]) {
		t.Fatal("normalization mismatch")
	}
	if NormalizeRecoveryCode("short") != "" {
		t.Fatal("invalid code normalized")
	}
}

func TestURI(t *testing.T) {
	uri := URI("Mini Server", "admin", "ABC")
	if !strings.HasPrefix(uri, "otpauth://totp/Mini%20Server:admin?") || !strings.Contains(uri, "secret=ABC") {
		t.Fatalf("unexpected uri %q", uri)
	}
}
