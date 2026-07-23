package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/kantaevsherhan/mini-ubuntu-server-panel/backend/internal/updater"
)

type fakeUpdateChecker struct{}

func (fakeUpdateChecker) Check(context.Context, string) (updater.Status, error) {
	return updater.Status{Current: "v1.0.0", Latest: "v1.1.0", Available: true, URL: "https://github.com/kantaevsherhan/mini-ubuntu-server-panel/releases/tag/v1.1.0"}, nil
}

func TestSettingsOverviewAndUpdateStatus(t *testing.T) {
	db, token := testAuthorizedDB(t)
	app := fiber.New()
	API{DB: db, Secret: "test-secret-that-is-long-enough", Version: "v1.0.0", DataDir: t.TempDir(), LogDir: t.TempDir(), Updates: fakeUpdateChecker{}}.Register(app)

	overviewRequest := httptest.NewRequest(http.MethodGet, "/api/v1/settings/overview", nil)
	overviewRequest.Header.Set(fiber.HeaderAuthorization, "Bearer "+token)
	overviewResponse, err := app.Test(overviewRequest)
	if err != nil || overviewResponse.StatusCode != http.StatusOK {
		t.Fatalf("settings overview failed: status=%d err=%v", overviewResponse.StatusCode, err)
	}
	var overview map[string]any
	if json.NewDecoder(overviewResponse.Body).Decode(&overview) != nil || overview["version"] != "v1.0.0" {
		t.Fatalf("unexpected settings overview: %#v", overview)
	}

	updateRequest := httptest.NewRequest(http.MethodGet, "/api/v1/updates", nil)
	updateRequest.Header.Set(fiber.HeaderAuthorization, "Bearer "+token)
	updateResponse, err := app.Test(updateRequest)
	if err != nil || updateResponse.StatusCode != http.StatusOK {
		t.Fatalf("update status failed: status=%d err=%v", updateResponse.StatusCode, err)
	}
}
