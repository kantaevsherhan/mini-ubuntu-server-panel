package auth

import (
	"errors"
	"strconv"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

type Claims struct {
	UserID   int64  `json:"uid"`
	Username string `json:"usr"`
	Role     string `json:"role"`
	jwt.RegisteredClaims
}

func Hash(password string) (string, error) {
	b, e := bcrypt.GenerateFromPassword([]byte(password), 12)
	return string(b), e
}
func Verify(hash, password string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}
func Sign(secret string, id int64, username, role string) (token, sessionID string, expiresAt time.Time, err error) {
	now := time.Now().UTC()
	expiresAt = now.Add(8 * time.Hour)
	sessionID = uuid.NewString()
	c := Claims{id, username, role, jwt.RegisteredClaims{ID: sessionID, IssuedAt: jwt.NewNumericDate(now), ExpiresAt: jwt.NewNumericDate(expiresAt)}}
	token, err = jwt.NewWithClaims(jwt.SigningMethodHS256, c).SignedString([]byte(secret))
	return token, sessionID, expiresAt, err
}
func Parse(secret, token string) (*Claims, error) {
	t, err := jwt.ParseWithClaims(token, &Claims{}, func(t *jwt.Token) (any, error) {
		if t.Method != jwt.SigningMethodHS256 {
			return nil, errors.New("invalid signing method")
		}
		return []byte(secret), nil
	})
	if err != nil || !t.Valid {
		return nil, errors.New("invalid token")
	}
	return t.Claims.(*Claims), nil
}

// ChallengeTTL bounds how long a password-verified login may wait for its second factor.
const ChallengeTTL = 5 * time.Minute

// challengeKey separates second-factor challenge tokens from access tokens: a challenge
// signed with it can never pass Parse, and an access token can never pass ParseChallenge.
func challengeKey(secret string) []byte {
	return []byte(secret + "\x00mfa-challenge")
}

// SignChallenge issues a short-lived token proving the password step succeeded.
func SignChallenge(secret string, id int64) (string, error) {
	now := time.Now().UTC()
	c := jwt.RegisteredClaims{ID: uuid.NewString(), Subject: strconv.FormatInt(id, 10), Audience: jwt.ClaimStrings{"mfa"}, IssuedAt: jwt.NewNumericDate(now), ExpiresAt: jwt.NewNumericDate(now.Add(ChallengeTTL))}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, c).SignedString(challengeKey(secret))
}

// ParseChallenge returns the user ID and challenge ID of a valid challenge token.
func ParseChallenge(secret, token string) (int64, string, error) {
	var claims jwt.RegisteredClaims
	t, err := jwt.ParseWithClaims(token, &claims, func(t *jwt.Token) (any, error) {
		if t.Method != jwt.SigningMethodHS256 {
			return nil, errors.New("invalid signing method")
		}
		return challengeKey(secret), nil
	}, jwt.WithAudience("mfa"))
	if err != nil || !t.Valid {
		return 0, "", errors.New("invalid challenge")
	}
	id, err := strconv.ParseInt(claims.Subject, 10, 64)
	if err != nil || id <= 0 || claims.ID == "" {
		return 0, "", errors.New("invalid challenge")
	}
	return id, claims.ID, nil
}
