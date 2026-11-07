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

func TestDatabaseClientEndpointsRequireAdminAndValidate(t *testing.T) {
	db, token := testAuthorizedDB(t)
	app := fiber.New()
	API{DB: db, Secret: "test-secret-that-is-long-enough"}.Register(app)
	send := func(path, body, auth string) int {
		req := httptest.NewRequest("POST", path, bytes.NewBufferString(body))
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
	invalid := `{"engine":"postgres","host":"127.0.0.1","port":5432,"database":"app; DROP"}`
	for _, op := range []string{"overview", "structure", "select", "query", "rows"} {
		path := "/api/v1/databases/" + op
		if got := send(path, invalid, ""); got != 401 {
			t.Fatalf("%s anonymous: %d", op, got)
		}
		if got := send(path, invalid, token); got != 422 {
			t.Fatalf("%s invalid: %d", op, got)
		}
	}
	// Without a Docker controller a container reference cannot be resolved.
	if got := send("/api/v1/databases/overview", `{"container":"abcdefabcdef"}`, token); got != 404 {
		t.Fatalf("container: %d", got)
	}
	req := httptest.NewRequest("GET", "/api/v1/databases/containers", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	res, err := app.Test(req)
	if err != nil || res.StatusCode != 200 {
		t.Fatalf("containers: %v", err)
	}
	if err := db.Model(&database.User{}).Where("username = ?", "admin").Update("role", "operator").Error; err != nil {
		t.Fatal(err)
	}
	if got := send("/api/v1/databases/query", `{"engine":"postgres","host":"127.0.0.1","port":5432,"database":"app","sql":"SELECT 1"}`, token); got == 200 || got == 422 {
		t.Fatalf("operator accepted: %d", got)
	}
}
