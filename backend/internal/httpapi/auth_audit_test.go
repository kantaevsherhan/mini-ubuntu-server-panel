package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/kantaevsherhan/mini-ubuntu-server-panel/backend/internal/database"
)

func TestLogoutWritesAudit(t *testing.T) {
	db, token := testAuthorizedDB(t)
	app := fiber.New()
	API{DB: db, Secret: "test-secret-that-is-long-enough"}.Register(app)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil)
	request.Header.Set(fiber.HeaderAuthorization, "Bearer "+token)
	response, err := app.Test(request)
	if err != nil || response.StatusCode != http.StatusNoContent {
		t.Fatalf("logout failed: status=%d err=%v", response.StatusCode, err)
	}
	var count int64
	if err := db.Model(&database.AuditEvent{}).Where("action = ? AND target_type = ?", "auth.logout", "web_session").Count(&count).Error; err != nil || count != 1 {
		t.Fatalf("logout audit missing: count=%d err=%v", count, err)
	}
}
