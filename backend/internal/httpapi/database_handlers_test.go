package httpapi

import (
	"bytes"
	"github.com/gofiber/fiber/v2"
	"github.com/kantaevsherhan/mini-ubuntu-server-panel/backend/internal/database"
	"net/http/httptest"
	"testing"
)

func TestDatabaseBrowserAuthorizationAndValidation(t *testing.T) {
	db, token := testAuthorizedDB(t)
	app := fiber.New()
	API{DB: db, Secret: "test-secret-that-is-long-enough"}.Register(app)
	send := func(auth string) int {
		req := httptest.NewRequest("POST", "/api/v1/databases/browse", bytes.NewBufferString(`{"engine":"sqlite","host":"/etc/shadow"}`))
		req.Header.Set("Content-Type", "application/json")
		if auth != "" {
			req.Header.Set("Authorization", "Bearer "+auth)
		}
		res, err := app.Test(req)
		if err != nil {
			t.Fatal(err)
		}
		return res.StatusCode
	}
	if got := send(""); got != 401 {
		t.Fatalf("anonymous: %d", got)
	}
	if got := send(token); got != 422 {
		t.Fatalf("invalid: %d", got)
	}
	if err := db.Model(&database.User{}).Where("username = ?", "admin").Update("role", "operator").Error; err != nil {
		t.Fatal(err)
	}
	if got := send(token); got == 200 || got == 422 {
		t.Fatalf("operator accepted: %d", got)
	}
}
