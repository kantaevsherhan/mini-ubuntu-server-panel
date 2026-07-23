package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/kantaevsherhan/mini-ubuntu-server-panel/backend/internal/database"
)

func TestUpdateNotificationRuleValidatesAndPersistsRecipients(t *testing.T) {
	db, token := testAuthorizedDB(t)
	recipient := database.TelegramRecipient{TelegramChatID: 42, Enabled: true, ReceiveAlerts: true, CreatedAt: time.Now().UTC()}
	if err := db.Create(&recipient).Error; err != nil {
		t.Fatal(err)
	}
	app := fiber.New()
	API{DB: db, Secret: "test-secret-that-is-long-enough"}.Register(app)
	payload, _ := json.Marshal(map[string]any{
		"enabled": true, "severity": "critical", "recipient_ids": []int64{recipient.ID},
		"cooldown_seconds": 900, "repeat_interval_seconds": 1800, "send_recovery": true,
	})
	request := httptest.NewRequest(http.MethodPut, "/api/v1/notifications/rules/resource.cpu.high", bytes.NewReader(payload))
	request.Header.Set(fiber.HeaderContentType, fiber.MIMEApplicationJSON)
	request.Header.Set(fiber.HeaderAuthorization, "Bearer "+token)
	response, err := app.Test(request)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", response.StatusCode)
	}
	var rule database.NotificationRule
	if err := db.First(&rule, "event_key = ?", "resource.cpu.high").Error; err != nil {
		t.Fatal(err)
	}
	if rule.Severity != "critical" || rule.CooldownSeconds != 900 || !rule.SendRecovery {
		t.Fatalf("unexpected rule: %#v", rule)
	}
	var count int64
	if err := db.Model(&database.NotificationRuleRecipient{}).Where("event_key = ? AND recipient_id = ?", rule.EventKey, recipient.ID).Count(&count).Error; err != nil || count != 1 {
		t.Fatalf("recipient link missing: count=%d err=%v", count, err)
	}
}
