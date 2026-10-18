// Package totp implements RFC 6238 time-based one-time passwords (SHA-1, 6 digits,
// 30-second steps — the profile every authenticator app supports), sealed secret
// storage and single-use recovery codes. It depends only on the standard library.
package totp

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base32"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"
)

const (
	Period = 30
	Digits = 6
	// Skew accepts the previous and next step to tolerate clock drift.
	Skew              = 1
	secretBytes       = 20
	RecoveryCodeCount = 10
)

var encoding = base32.StdEncoding.WithPadding(base32.NoPadding)

// NewSecret returns a random base32 secret suitable for authenticator apps.
func NewSecret() (string, error) {
	raw := make([]byte, secretBytes)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return encoding.EncodeToString(raw), nil
}

// URI builds the otpauth:// provisioning URI encoded into the enrollment QR code.
func URI(issuer, account, secret string) string {
	label := url.PathEscape(issuer + ":" + account)
	query := url.Values{}
	query.Set("secret", secret)
	query.Set("issuer", issuer)
	query.Set("algorithm", "SHA1")
	query.Set("digits", fmt.Sprint(Digits))
	query.Set("period", fmt.Sprint(Period))
	return "otpauth://totp/" + label + "?" + query.Encode()
}

// Step returns the RFC 6238 counter for a moment in time.
func Step(at time.Time) int64 {
	return at.Unix() / Period
}

// Code computes the one-time password for a counter.
func Code(secret string, step int64) (string, error) {
	key, err := encoding.DecodeString(strings.ToUpper(strings.TrimSpace(secret)))
	if err != nil || len(key) == 0 {
		return "", errors.New("invalid totp secret")
	}
	var counter [8]byte
	binary.BigEndian.PutUint64(counter[:], uint64(step))
	mac := hmac.New(sha1.New, key)
	mac.Write(counter[:])
	sum := mac.Sum(nil)
	offset := sum[len(sum)-1] & 0x0f
	value := binary.BigEndian.Uint32(sum[offset:offset+4]) & 0x7fffffff
	return fmt.Sprintf("%06d", value%1_000_000), nil
}

// Verify checks a code within the skew window and returns the matched step.
// Steps at or below lastStep are rejected so a code cannot be replayed.
func Verify(secret, code string, at time.Time, lastStep int64) (int64, bool) {
	code = strings.TrimSpace(code)
	if len(code) != Digits {
		return 0, false
	}
	current := Step(at)
	for delta := -Skew; delta <= Skew; delta++ {
		step := current + int64(delta)
		if step <= lastStep {
			continue
		}
		expected, err := Code(secret, step)
		if err == nil && subtle.ConstantTimeCompare([]byte(expected), []byte(code)) == 1 {
			return step, true
		}
	}
	return 0, false
}

// Sealer encrypts secrets at rest with AES-256-GCM using a key derived from the
// panel's server secret, so a leaked database alone does not reveal TOTP seeds.
type Sealer struct {
	aead cipher.AEAD
}

func NewSealer(serverSecret string) (*Sealer, error) {
	if len(serverSecret) < 16 {
		return nil, errors.New("server secret too short")
	}
	mac := hmac.New(sha256.New, []byte(serverSecret))
	mac.Write([]byte("mini-ubuntu-server/totp/v1"))
	block, err := aes.NewCipher(mac.Sum(nil))
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &Sealer{aead: aead}, nil
}

func (s *Sealer) Seal(plain string) (string, error) {
	nonce := make([]byte, s.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(s.aead.Seal(nonce, nonce, []byte(plain), nil)), nil
}

func (s *Sealer) Open(sealed string) (string, error) {
	data, err := base64.StdEncoding.DecodeString(sealed)
	if err != nil || len(data) < s.aead.NonceSize() {
		return "", errors.New("invalid sealed value")
	}
	plain, err := s.aead.Open(nil, data[:s.aead.NonceSize()], data[s.aead.NonceSize():], nil)
	if err != nil {
		return "", errors.New("invalid sealed value")
	}
	return string(plain), nil
}

// NewRecoveryCodes returns human-friendly single-use codes (xxxxx-xxxxx, 50 bits each).
func NewRecoveryCodes() ([]string, error) {
	codes := make([]string, RecoveryCodeCount)
	for i := range codes {
		raw := make([]byte, 7)
		if _, err := rand.Read(raw); err != nil {
			return nil, err
		}
		value := strings.ToLower(encoding.EncodeToString(raw))[:10]
		codes[i] = value[:5] + "-" + value[5:]
	}
	return codes, nil
}

// NormalizeRecoveryCode accepts codes with or without the dash and in any case.
func NormalizeRecoveryCode(code string) string {
	code = strings.ToLower(strings.TrimSpace(code))
	code = strings.ReplaceAll(code, "-", "")
	code = strings.ReplaceAll(code, " ", "")
	if len(code) != 10 {
		return ""
	}
	return code[:5] + "-" + code[5:]
}

// HashRecoveryCode stores codes as SHA-256: they are random with 50 bits of entropy,
// so a slow password hash is unnecessary and would make login checks expensive.
func HashRecoveryCode(code string) string {
	sum := sha256.Sum256([]byte(NormalizeRecoveryCode(code)))
	return hex.EncodeToString(sum[:])
}
