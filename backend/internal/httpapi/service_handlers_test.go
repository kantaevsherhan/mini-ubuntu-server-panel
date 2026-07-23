package httpapi

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/kantaevsherhan/mini-ubuntu-server-panel/backend/internal/database"
	"github.com/kantaevsherhan/mini-ubuntu-server-panel/backend/internal/services"
)

type fakeServices struct {
	unit   string
	action string
}

func (f *fakeServices) List(context.Context) ([]services.Service, error) {
	return []services.Service{{Name: "ssh.service", ActiveState: "active", Enabled: "enabled"}}, nil
}

func (f *fakeServices) Action(_ context.Context, unit, action string) error {
	f.unit, f.action = unit, action
	return nil
}

func TestServiceRoutesListActionAndAudit(t *testing.T) {
	db, token := testAuthorizedDB(t)
	controller := &fakeServices{}
	app := fiber.New()
	API{DB: db, Services: controller, Secret: "test-secret-that-is-long-enough"}.Register(app)

	listRequest := httptest.NewRequest(http.MethodGet, "/api/v1/services", nil)
	listRequest.Header.Set(fiber.HeaderAuthorization, "Bearer "+token)
	listResponse, err := app.Test(listRequest)
	if err != nil || listResponse.StatusCode != http.StatusOK {
		t.Fatalf("list failed: status=%d err=%v", listResponse.StatusCode, err)
	}

	actionRequest := httptest.NewRequest(http.MethodPost, "/api/v1/services/ssh.service/action", bytes.NewBufferString(`{"action":"restart"}`))
	actionRequest.Header.Set(fiber.HeaderAuthorization, "Bearer "+token)
	actionRequest.Header.Set(fiber.HeaderContentType, fiber.MIMEApplicationJSON)
	actionResponse, err := app.Test(actionRequest)
	if err != nil || actionResponse.StatusCode != http.StatusNoContent {
		t.Fatalf("action failed: status=%d err=%v", actionResponse.StatusCode, err)
	}
	if controller.unit != "ssh.service" || controller.action != "restart" {
		t.Fatalf("unexpected action: unit=%q action=%q", controller.unit, controller.action)
	}
	var count int64
	if err := db.Model(&database.AuditEvent{}).Where("action = ? AND target_type = ?", "service.restart", "systemd_service").Count(&count).Error; err != nil || count != 1 {
		t.Fatalf("audit record missing: count=%d err=%v", count, err)
	}
}
