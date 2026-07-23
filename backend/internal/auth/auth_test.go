package auth

import (
	"testing"
	"time"
)

func TestPasswordHashAndVerify(t *testing.T) {
	hash, err := Hash("correct horse battery staple")
	if err != nil {
		t.Fatal(err)
	}
	if !Verify(hash, "correct horse battery staple") {
		t.Fatal("expected password to verify")
	}
	if Verify(hash, "incorrect password") {
		t.Fatal("unexpected verification for wrong password")
	}
}

func TestJWTContainsSessionAndExpires(t *testing.T) {
	secret := "01234567890123456789012345678901"
	token, sessionID, expiresAt, err := Sign(secret, 42, "admin", "admin")
	if err != nil {
		t.Fatal(err)
	}
	claims, err := Parse(secret, token)
	if err != nil {
		t.Fatal(err)
	}
	if claims.UserID != 42 || claims.ID != sessionID || claims.Role != "admin" {
		t.Fatalf("unexpected claims: %#v", claims)
	}
	if expiresAt.Before(time.Now().Add(7 * time.Hour)) {
		t.Fatal("token expiration is too short")
	}
}

func TestJWTRejectsWrongSecret(t *testing.T) {
	token, _, _, err := Sign("01234567890123456789012345678901", 1, "admin", "admin")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Parse("different-secret-different-secret-000", token); err == nil {
		t.Fatal("expected signature verification failure")
	}
}
