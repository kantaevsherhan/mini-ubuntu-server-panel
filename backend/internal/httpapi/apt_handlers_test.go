package httpapi

import (
	"context"
	"net/http"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/kantaevsherhan/mini-ubuntu-server-panel/backend/internal/aptupdates"
	"github.com/kantaevsherhan/mini-ubuntu-server-panel/backend/internal/database"
)

type fakeApt struct {
	started []aptupdates.StartRequest
	busy    bool
}

func (f *fakeApt) Status(context.Context) (aptupdates.Status, error) {
	return aptupdates.Status{Packages: []aptupdates.Package{{Name: "openssl", Security: true}}, SecurityCount: 1, Job: aptupdates.Job{State: "idle", Log: []string{}}}, nil
}
func (f *fakeApt) Job(context.Context) (aptupdates.Job, error) {
	return aptupdates.Job{State: "running", Operation: "upgrade", Log: []string{"Unpacking openssl"}}, nil
}
func (f *fakeApt) Start(_ context.Context, request aptupdates.StartRequest) error {
	if f.busy {
		return aptupdates.ErrBusy
	}
	if request.Operation == "upgrade" {
		for _, name := range request.Packages {
			if name == "" || name[0] == '-' {
				return aptupdates.ErrInvalidRequest
			}
		}
	}
	f.started = append(f.started, request)
	return nil
}

func TestAptRoutesStartJobsAndAudit(t *testing.T) {
	db, token := testAuthorizedDB(t)
	controller := &fakeApt{}
	app := fiber.New()
	API{DB: db, Apt: controller, Secret: "test-secret-that-is-long-enough"}.Register(app)

	if status, body := callJSON(t, app, http.MethodGet, "/api/v1/apt", token, nil); status != http.StatusOK || body["security_count"] != float64(1) {
		t.Fatalf("status failed: %d %v", status, body)
	}
	if status, body := callJSON(t, app, http.MethodGet, "/api/v1/apt/job", token, nil); status != http.StatusOK || body["state"] != "running" {
		t.Fatalf("job failed: %d %v", status, body)
	}
	if status, _ := callJSON(t, app, http.MethodPost, "/api/v1/apt/refresh", token, nil); status != http.StatusAccepted {
		t.Fatalf("refresh failed: %d", status)
	}
	if status, _ := callJSON(t, app, http.MethodPost, "/api/v1/apt/upgrade", token, map[string][]string{"packages": {"openssl"}}); status != http.StatusAccepted {
		t.Fatalf("selective upgrade failed: %d", status)
	}
	if status, _ := callJSON(t, app, http.MethodPost, "/api/v1/apt/upgrade", token, map[string][]string{"packages": {"-oFoo"}}); status != http.StatusUnprocessableEntity {
		t.Fatalf("unsafe package accepted: %d", status)
	}
	if len(controller.started) != 2 || controller.started[0].Operation != "refresh" || controller.started[1].Packages[0] != "openssl" {
		t.Fatalf("unexpected jobs: %#v", controller.started)
	}
	controller.busy = true
	if status, body := callJSON(t, app, http.MethodPost, "/api/v1/apt/upgrade", token, nil); status != http.StatusConflict || body["error"] != "apt_job_running" {
		t.Fatalf("busy job not reported: %d %v", status, body)
	}
	var count int64
	db.Model(&database.AuditEvent{}).Where("action LIKE ?", "apt.%").Count(&count)
	if count != 2 {
		t.Fatalf("expected 2 apt audit records, got %d", count)
	}
}
