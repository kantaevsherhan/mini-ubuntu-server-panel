package httpapi

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/kantaevsherhan/mini-ubuntu-server-panel/backend/internal/database"
	dockermanager "github.com/kantaevsherhan/mini-ubuntu-server-panel/backend/internal/docker"
)

type fakeDocker struct {
	dockermanager.Controller
	id     string
	action string
}

func (f *fakeDocker) List(context.Context) ([]dockermanager.Container, error) {
	return []dockermanager.Container{{ID: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef", Name: "web", State: "running"}}, nil
}
func (f *fakeDocker) Action(_ context.Context, id, action string) error {
	f.id, f.action = id, action
	return nil
}

func TestDockerRoutesListActionAndAudit(t *testing.T) {
	db, token := testAuthorizedDB(t)
	controller := &fakeDocker{}
	app := fiber.New()
	API{DB: db, Docker: controller, Secret: "test-secret-that-is-long-enough"}.Register(app)

	listRequest := httptest.NewRequest(http.MethodGet, "/api/v1/docker/containers", nil)
	listRequest.Header.Set(fiber.HeaderAuthorization, "Bearer "+token)
	listResponse, err := app.Test(listRequest)
	if err != nil || listResponse.StatusCode != http.StatusOK {
		t.Fatalf("list failed: status=%d err=%v", listResponse.StatusCode, err)
	}

	id := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	actionRequest := httptest.NewRequest(http.MethodPost, "/api/v1/docker/containers/"+id+"/action", bytes.NewBufferString(`{"action":"restart"}`))
	actionRequest.Header.Set(fiber.HeaderAuthorization, "Bearer "+token)
	actionRequest.Header.Set(fiber.HeaderContentType, fiber.MIMEApplicationJSON)
	actionResponse, err := app.Test(actionRequest)
	if err != nil || actionResponse.StatusCode != http.StatusNoContent {
		t.Fatalf("action failed: status=%d err=%v", actionResponse.StatusCode, err)
	}
	if controller.id != id || controller.action != "restart" {
		t.Fatalf("unexpected action: id=%q action=%q", controller.id, controller.action)
	}
	var count int64
	if err := db.Model(&database.AuditEvent{}).Where("action = ? AND target_type = ?", "docker.container.restart", "docker_container").Count(&count).Error; err != nil || count != 1 {
		t.Fatalf("audit record missing: count=%d err=%v", count, err)
	}
}

func (f *fakeDocker) Run(_ context.Context, request dockermanager.RunRequest) (string, error) {
	f.action = "run:" + request.Image
	return "abcdefabcdef", nil
}

func TestDockerRunRequiresAdminAndValidatesInput(t *testing.T) {
	db, token := testAuthorizedDB(t)
	controller := &fakeDocker{}
	app := fiber.New()
	API{DB: db, Docker: controller, Secret: "test-secret-that-is-long-enough"}.Register(app)

	send := func(body string) int {
		request := httptest.NewRequest(http.MethodPost, "/api/v1/docker/containers", bytes.NewBufferString(body))
		request.Header.Set(fiber.HeaderAuthorization, "Bearer "+token)
		request.Header.Set(fiber.HeaderContentType, fiber.MIMEApplicationJSON)
		response, err := app.Test(request)
		if err != nil {
			t.Fatal(err)
		}
		return response.StatusCode
	}
	if status := send(`{"image":"nginx","volumes":[{"source":"/","target":"/host"}]}`); status != http.StatusUnprocessableEntity {
		t.Fatalf("unsafe bind mount accepted: status=%d", status)
	}
	if status := send(`{"image":"nginx:1.27","ports":[{"host_port":8080,"container_port":80,"protocol":"tcp"}],"start":true}`); status != http.StatusCreated {
		t.Fatalf("run failed: status=%d", status)
	}
	if controller.action != "run:nginx:1.27" {
		t.Fatalf("controller not called: %q", controller.action)
	}
	var count int64
	if err := db.Model(&database.AuditEvent{}).Where("action = ?", "docker.container.create").Count(&count).Error; err != nil || count != 1 {
		t.Fatalf("audit record missing: count=%d err=%v", count, err)
	}
}
