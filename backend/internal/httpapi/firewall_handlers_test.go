package httpapi

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/kantaevsherhan/mini-ubuntu-server-panel/backend/internal/database"
	"github.com/kantaevsherhan/mini-ubuntu-server-panel/backend/internal/firewall"
)

type fakeFirewall struct {
	added   *firewall.AddRequest
	deleted int
}

func (f *fakeFirewall) Status(context.Context) (firewall.Status, error) {
	return firewall.Status{Active: true, Rules: []firewall.Rule{{Number: 1, To: "22/tcp", Action: "allow", Direction: "in", From: "Anywhere"}}}, nil
}
func (f *fakeFirewall) Add(_ context.Context, request firewall.AddRequest) error {
	f.added = &request
	return nil
}
func (f *fakeFirewall) Delete(_ context.Context, number int) error {
	f.deleted = number
	return nil
}

func TestFirewallRoutesMutateAndAudit(t *testing.T) {
	db, token := testAuthorizedDB(t)
	controller := &fakeFirewall{}
	app := fiber.New()
	API{DB: db, Firewall: controller, Secret: "test-secret-that-is-long-enough"}.Register(app)

	addRequest := httptest.NewRequest(http.MethodPost, "/api/v1/firewall/rules", bytes.NewBufferString(`{"action":"allow","port":443,"protocol":"tcp","source":"any"}`))
	addRequest.Header.Set(fiber.HeaderAuthorization, "Bearer "+token)
	addRequest.Header.Set(fiber.HeaderContentType, fiber.MIMEApplicationJSON)
	addResponse, err := app.Test(addRequest)
	if err != nil || addResponse.StatusCode != http.StatusNoContent || controller.added == nil || controller.added.Port != 443 {
		t.Fatalf("add failed: status=%d request=%#v err=%v", addResponse.StatusCode, controller.added, err)
	}

	deleteRequest := httptest.NewRequest(http.MethodDelete, "/api/v1/firewall/rules/2", nil)
	deleteRequest.Header.Set(fiber.HeaderAuthorization, "Bearer "+token)
	deleteResponse, err := app.Test(deleteRequest)
	if err != nil || deleteResponse.StatusCode != http.StatusNoContent || controller.deleted != 2 {
		t.Fatalf("delete failed: status=%d number=%d err=%v", deleteResponse.StatusCode, controller.deleted, err)
	}
	var count int64
	if err := db.Model(&database.AuditEvent{}).Where("action LIKE ?", "firewall.rule.%").Count(&count).Error; err != nil || count != 2 {
		t.Fatalf("audit records missing: count=%d err=%v", count, err)
	}
}

func TestFirewallRejectsDenySSH(t *testing.T) {
	db, token := testAuthorizedDB(t)
	app := fiber.New()
	appController := &fakeFirewall{}
	API{DB: db, Firewall: appController, Secret: "test-secret-that-is-long-enough"}.Register(app)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/firewall/rules", bytes.NewBufferString(`{"action":"deny","port":22,"protocol":"tcp","source":"any"}`))
	request.Header.Set(fiber.HeaderAuthorization, "Bearer "+token)
	request.Header.Set(fiber.HeaderContentType, fiber.MIMEApplicationJSON)
	response, err := app.Test(request)
	if err != nil || response.StatusCode != http.StatusUnprocessableEntity || appController.added != nil {
		t.Fatalf("unsafe SSH rule was not rejected: status=%d err=%v", response.StatusCode, err)
	}
}
