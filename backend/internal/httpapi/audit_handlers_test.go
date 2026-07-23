package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/kantaevsherhan/mini-ubuntu-server-panel/backend/internal/database"
)

func TestAuditResponseIncludesActorAndRequestMetadata(t *testing.T) {
	db, token := testAuthorizedDB(t)
	actorID := int64(1)
	targetID := "service.example"
	event := database.AuditEvent{
		ActorUserID: &actorID, Action: "service.restart", TargetType: "systemd",
		TargetID: &targetID, DetailsJSON: `{}`, IPAddress: "192.0.2.1",
	}
	if err := db.Create(&event).Error; err != nil {
		t.Fatal(err)
	}
	app := fiber.New()
	API{DB: db, Secret: "test-secret-that-is-long-enough"}.Register(app)
	request := httptest.NewRequest(http.MethodGet, "/api/v1/audit", nil)
	request.Header.Set(fiber.HeaderAuthorization, "Bearer "+token)
	response, err := app.Test(request)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", response.StatusCode)
	}
	var payload []struct {
		ActorUserID *int64 `json:"actor_user_id"`
		IPAddress   string `json:"ip_address"`
		Action      string `json:"action"`
	}
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if len(payload) != 1 || payload[0].ActorUserID == nil || *payload[0].ActorUserID != actorID || payload[0].IPAddress != "192.0.2.1" || payload[0].Action != "service.restart" {
		t.Fatalf("unexpected audit response: %#v", payload)
	}
}
