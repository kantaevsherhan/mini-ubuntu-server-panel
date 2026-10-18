package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/kantaevsherhan/mini-ubuntu-server-panel/backend/internal/database"
	"github.com/kantaevsherhan/mini-ubuntu-server-panel/backend/internal/totp"
)

func callJSON(t *testing.T, app *fiber.App, method, path, token string, body any) (int, map[string]any) {
	t.Helper()
	payload, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(method, path, bytes.NewReader(payload))
	request.Header.Set(fiber.HeaderContentType, fiber.MIMEApplicationJSON)
	if token != "" {
		request.Header.Set(fiber.HeaderAuthorization, "Bearer "+token)
	}
	response, err := app.Test(request, 5000)
	if err != nil {
		t.Fatal(err)
	}
	result := map[string]any{}
	_ = json.NewDecoder(response.Body).Decode(&result)
	return response.StatusCode, result
}

func TestTwoFactorEnrollmentLoginAndRecovery(t *testing.T) {
	db, token := testAuthorizedDB(t)
	app := fiber.New()
	API{DB: db, Secret: "test-secret-that-is-long-enough"}.Register(app)

	if status, _ := callJSON(t, app, http.MethodPost, "/api/v1/auth/2fa/setup", token, map[string]string{"password": "wrong"}); status != http.StatusUnauthorized {
		t.Fatalf("setup with wrong password: status %d", status)
	}
	status, setup := callJSON(t, app, http.MethodPost, "/api/v1/auth/2fa/setup", token, map[string]string{"password": "admin-test-password"})
	secret, _ := setup["secret"].(string)
	if status != http.StatusOK || secret == "" || !strings.HasPrefix(setup["otpauth_uri"].(string), "otpauth://totp/") {
		t.Fatalf("setup failed: %d %v", status, setup)
	}
	var stored database.UserTOTP
	if err := db.First(&stored).Error; err != nil || stored.PendingSecretSealed == nil || strings.Contains(*stored.PendingSecretSealed, secret) {
		t.Fatalf("pending secret not sealed: %+v err=%v", stored, err)
	}

	if status, _ := callJSON(t, app, http.MethodPost, "/api/v1/auth/2fa/enable", token, map[string]string{"code": "000000"}); status != http.StatusUnprocessableEntity {
		t.Fatalf("enable with wrong code: status %d", status)
	}
	now := time.Now()
	code, _ := totp.Code(secret, totp.Step(now)-1)
	status, enabled := callJSON(t, app, http.MethodPost, "/api/v1/auth/2fa/enable", token, map[string]string{"code": code})
	codes, _ := enabled["recovery_codes"].([]any)
	if status != http.StatusOK || len(codes) != totp.RecoveryCodeCount {
		t.Fatalf("enable failed: %d %v", status, enabled)
	}

	login := map[string]string{"username": "admin", "password": "admin-test-password"}
	status, first := callJSON(t, app, http.MethodPost, "/api/v1/auth/login", "", login)
	challenge, _ := first["mfa_token"].(string)
	if status != http.StatusOK || first["mfa_required"] != true || challenge == "" || first["access_token"] != nil {
		t.Fatalf("password step must not issue a session: %d %v", status, first)
	}
	if status, _ := callJSON(t, app, http.MethodGet, "/api/v1/dashboard", challenge, nil); status != http.StatusUnauthorized {
		t.Fatalf("challenge token accepted as session: %d", status)
	}
	// The enrollment code's step was consumed, so replaying it must fail.
	if status, _ := callJSON(t, app, http.MethodPost, "/api/v1/auth/login/mfa", "", map[string]string{"mfa_token": challenge, "code": code}); status != http.StatusUnauthorized {
		t.Fatalf("replayed enrollment code accepted: %d", status)
	}
	current, _ := totp.Code(secret, totp.Step(now)+1)
	status, second := callJSON(t, app, http.MethodPost, "/api/v1/auth/login/mfa", "", map[string]string{"mfa_token": challenge, "code": current})
	if status != http.StatusOK || second["access_token"] == nil {
		t.Fatalf("second factor login failed: %d %v", status, second)
	}
	if status, _ := callJSON(t, app, http.MethodPost, "/api/v1/auth/login/mfa", "", map[string]string{"mfa_token": challenge, "code": codes[0].(string)}); status != http.StatusUnauthorized {
		t.Fatalf("challenge reused after success: %d", status)
	}

	_, again := callJSON(t, app, http.MethodPost, "/api/v1/auth/login", "", login)
	recovery := strings.ToUpper(strings.ReplaceAll(codes[0].(string), "-", ""))
	if status, body := callJSON(t, app, http.MethodPost, "/api/v1/auth/login/mfa", "", map[string]string{"mfa_token": again["mfa_token"].(string), "code": recovery}); status != http.StatusOK {
		t.Fatalf("recovery code login failed: %d %v", status, body)
	}
	_, third := callJSON(t, app, http.MethodPost, "/api/v1/auth/login", "", login)
	if status, _ := callJSON(t, app, http.MethodPost, "/api/v1/auth/login/mfa", "", map[string]string{"mfa_token": third["mfa_token"].(string), "code": codes[0].(string)}); status != http.StatusUnauthorized {
		t.Fatalf("used recovery code accepted twice: %d", status)
	}

	newToken := second["access_token"].(string)
	if status, _ := callJSON(t, app, http.MethodPost, "/api/v1/auth/2fa/disable", newToken, map[string]string{"password": "admin-test-password", "code": "000000"}); status != http.StatusUnauthorized {
		t.Fatalf("disable without valid code: %d", status)
	}
	if status, body := callJSON(t, app, http.MethodPost, "/api/v1/auth/2fa/disable", newToken, map[string]string{"password": "admin-test-password", "code": codes[1].(string)}); status != http.StatusNoContent {
		t.Fatalf("disable failed: %d %v", status, body)
	}
	if status, body := callJSON(t, app, http.MethodPost, "/api/v1/auth/login", "", login); status != http.StatusOK || body["access_token"] == nil {
		t.Fatalf("login after disable must be single-factor: %d %v", status, body)
	}
	var audits int64
	db.Model(&database.AuditEvent{}).Where("action IN ?", []string{"auth.mfa.enable", "auth.mfa.disable", "auth.mfa_failed"}).Count(&audits)
	if audits < 3 {
		t.Fatalf("expected mfa audit records, got %d", audits)
	}
}

func TestTwoFactorChallengeAttemptLimitAndAdminReset(t *testing.T) {
	db, token := testAuthorizedDB(t)
	app := fiber.New()
	API{DB: db, Secret: "test-secret-that-is-long-enough"}.Register(app)
	_, setup := callJSON(t, app, http.MethodPost, "/api/v1/auth/2fa/setup", token, map[string]string{"password": "admin-test-password"})
	secret := setup["secret"].(string)
	code, _ := totp.Code(secret, totp.Step(time.Now()))
	if status, _ := callJSON(t, app, http.MethodPost, "/api/v1/auth/2fa/enable", token, map[string]string{"code": code}); status != http.StatusOK {
		t.Fatalf("enable failed: %d", status)
	}
	if status, body := callJSON(t, app, http.MethodGet, "/api/v1/users", token, nil); status != http.StatusOK || body == nil {
		t.Fatalf("users list failed: %d", status)
	}

	_, first := callJSON(t, app, http.MethodPost, "/api/v1/auth/login", "", map[string]string{"username": "admin", "password": "admin-test-password"})
	challenge := first["mfa_token"].(string)
	for range maxChallengeAttempts {
		callJSON(t, app, http.MethodPost, "/api/v1/auth/login/mfa", "", map[string]string{"mfa_token": challenge, "code": "000000"})
	}
	next, _ := totp.Code(secret, totp.Step(time.Now())+1)
	if status, body := callJSON(t, app, http.MethodPost, "/api/v1/auth/login/mfa", "", map[string]string{"mfa_token": challenge, "code": next}); status != http.StatusUnauthorized || body["error"] != "mfa_challenge_invalid" {
		t.Fatalf("challenge must be locked after %d failures: %d %v", maxChallengeAttempts, status, body)
	}

	if status, _ := callJSON(t, app, http.MethodPost, "/api/v1/users/1/reset-2fa", token, nil); status != http.StatusNoContent {
		t.Fatalf("admin reset failed: %d", status)
	}
	var count int64
	db.Model(&database.UserTOTP{}).Count(&count)
	if count != 0 {
		t.Fatal("2FA record survived reset")
	}
	if status, _ := callJSON(t, app, http.MethodGet, "/api/v1/dashboard", token, nil); status != http.StatusUnauthorized {
		t.Fatalf("sessions must be revoked after reset: %d", status)
	}
}
