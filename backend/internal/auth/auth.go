package auth

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

type Claims struct { UserID int64 `json:"uid"`; Username string `json:"usr"`; Role string `json:"role"`; jwt.RegisteredClaims }

func Hash(password string) (string, error) { b, e := bcrypt.GenerateFromPassword([]byte(password), 12); return string(b), e }
func Verify(hash, password string) bool { return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil }
func Sign(secret string, id int64, username, role string) (string, error) {
	now := time.Now(); c := Claims{id, username, role, jwt.RegisteredClaims{IssuedAt: jwt.NewNumericDate(now), ExpiresAt: jwt.NewNumericDate(now.Add(8*time.Hour))}}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, c).SignedString([]byte(secret))
}
func Parse(secret, token string) (*Claims, error) {
	t, err := jwt.ParseWithClaims(token, &Claims{}, func(t *jwt.Token)(any,error){ if t.Method != jwt.SigningMethodHS256{return nil,errors.New("invalid signing method")}; return []byte(secret),nil })
	if err != nil || !t.Valid { return nil, errors.New("invalid token") }; return t.Claims.(*Claims), nil
}
