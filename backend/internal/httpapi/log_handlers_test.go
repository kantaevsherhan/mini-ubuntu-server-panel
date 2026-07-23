package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/kantaevsherhan/mini-ubuntu-server-panel/backend/internal/logs"
)

type fakeLogs struct {
	query logs.Query
}

func (f *fakeLogs) List(_ context.Context, query logs.Query) ([]logs.Entry, error) {
	f.query = query
	return []logs.Entry{{Timestamp: time.Unix(1, 0).UTC(), Unit: "ssh.service", Priority: "3", Message: "started"}}, nil
}

func TestLogsRouteValidatesAndLists(t *testing.T) {
	db, token := testAuthorizedDB(t)
	controller := &fakeLogs{}
	app := fiber.New()
	API{DB: db, Logs: controller, Secret: "test-secret-that-is-long-enough"}.Register(app)
	request := httptest.NewRequest(http.MethodGet, "/api/v1/logs?unit=ssh.service&priority=warning&range=hour&limit=250", nil)
	request.Header.Set(fiber.HeaderAuthorization, "Bearer "+token)
	response, err := app.Test(request)
	if err != nil || response.StatusCode != http.StatusOK || controller.query.Unit != "ssh.service" || controller.query.Limit != 250 {
		t.Fatalf("logs request failed: status=%d query=%#v err=%v", response.StatusCode, controller.query, err)
	}
}

func TestLogsRouteRejectsUnsafeUnit(t *testing.T) {
	db, token := testAuthorizedDB(t)
	controller := &fakeLogs{}
	app := fiber.New()
	API{DB: db, Logs: controller, Secret: "test-secret-that-is-long-enough"}.Register(app)
	request := httptest.NewRequest(http.MethodGet, "/api/v1/logs?unit=ssh.service%3Breboot", nil)
	request.Header.Set(fiber.HeaderAuthorization, "Bearer "+token)
	response, err := app.Test(request)
	if err != nil || response.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("unsafe query was not rejected: status=%d err=%v", response.StatusCode, err)
	}
}
