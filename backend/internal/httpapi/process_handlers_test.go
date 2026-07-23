package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/kantaevsherhan/mini-ubuntu-server-panel/backend/internal/database"
	"github.com/kantaevsherhan/mini-ubuntu-server-panel/backend/internal/processes"
)

type fakeProcesses struct {
	pid    int
	signal string
}

func (f *fakeProcesses) List(context.Context) ([]processes.Process, error) {
	return []processes.Process{{PID: 42, Name: "worker", Username: "server", State: "S", StartedAt: time.Unix(1, 0).UTC()}}, nil
}

func (f *fakeProcesses) Signal(_ context.Context, pid int, signal string) error {
	f.pid = pid
	f.signal = signal
	return nil
}

func TestProcessRoutesListSignalAndAudit(t *testing.T) {
	db, token := testAuthorizedDB(t)
	controller := &fakeProcesses{}
	app := fiber.New()
	API{DB: db, Processes: controller, Secret: "test-secret-that-is-long-enough"}.Register(app)

	listRequest := httptest.NewRequest(http.MethodGet, "/api/v1/processes", nil)
	listRequest.Header.Set(fiber.HeaderAuthorization, "Bearer "+token)
	listResponse, err := app.Test(listRequest)
	if err != nil || listResponse.StatusCode != http.StatusOK {
		t.Fatalf("list failed: status=%d err=%v", listResponse.StatusCode, err)
	}

	payload, _ := json.Marshal(map[string]string{"signal": "TERM"})
	signalRequest := httptest.NewRequest(http.MethodPost, "/api/v1/processes/42/signal", bytes.NewReader(payload))
	signalRequest.Header.Set(fiber.HeaderAuthorization, "Bearer "+token)
	signalRequest.Header.Set(fiber.HeaderContentType, fiber.MIMEApplicationJSON)
	signalResponse, err := app.Test(signalRequest)
	if err != nil || signalResponse.StatusCode != http.StatusNoContent {
		t.Fatalf("signal failed: status=%d err=%v", signalResponse.StatusCode, err)
	}
	if controller.pid != 42 || controller.signal != "TERM" {
		t.Fatalf("unexpected signal call: pid=%d signal=%q", controller.pid, controller.signal)
	}
	var count int64
	if err := db.Model(&database.AuditEvent{}).Where("action = ? AND target_id = ?", "process.signal", "42").Count(&count).Error; err != nil || count != 1 {
		t.Fatalf("audit record missing: count=%d err=%v", count, err)
	}
}

func TestProcessSignalRejectsUnsafeInput(t *testing.T) {
	db, token := testAuthorizedDB(t)
	controller := &fakeProcesses{}
	app := fiber.New()
	API{DB: db, Processes: controller, Secret: "test-secret-that-is-long-enough"}.Register(app)
	payload, _ := json.Marshal(map[string]string{"signal": "STOP"})
	request := httptest.NewRequest(http.MethodPost, "/api/v1/processes/1/signal", bytes.NewReader(payload))
	request.Header.Set(fiber.HeaderAuthorization, "Bearer "+token)
	request.Header.Set(fiber.HeaderContentType, fiber.MIMEApplicationJSON)
	response, err := app.Test(request)
	if err != nil || response.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("unsafe signal was not rejected: status=%d err=%v", response.StatusCode, err)
	}
}
