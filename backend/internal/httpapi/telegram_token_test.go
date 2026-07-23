package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/kantaevsherhan/mini-ubuntu-server-panel/backend/internal/database"
)

type fakeSecretWriter struct{ token string }

func (f *fakeSecretWriter) SetTelegramToken(_ context.Context, token string) error {
	f.token = token
	return nil
}

func TestUpdateTelegramTokenUsesSecretWriterAndRedactsAudit(t *testing.T) {
	db, token := testAuthorizedDB(t)
	writer := &fakeSecretWriter{}
	app := fiber.New()
	API{DB: db, Secrets: writer, Secret: "test-secret-that-is-long-enough"}.Register(app)
	telegramToken := "123456789:ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghi_123"
	payload, _ := json.Marshal(map[string]string{"token": telegramToken})
	request := httptest.NewRequest(http.MethodPut, "/api/v1/telegram/token", bytes.NewReader(payload))
	request.Header.Set(fiber.HeaderContentType, fiber.MIMEApplicationJSON)
	request.Header.Set(fiber.HeaderAuthorization, "Bearer "+token)
	response, err := app.Test(request)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusNoContent || writer.token != telegramToken {
		t.Fatalf("token update failed: status=%d", response.StatusCode)
	}
	var event database.AuditEvent
	if err := db.Where("action = ?", "telegram.token.update").First(&event).Error; err != nil {
		t.Fatal(err)
	}
	if strings.Contains(event.DetailsJSON, telegramToken) || !strings.Contains(event.DetailsJSON, `"token_value":"hidden"`) {
		t.Fatalf("unsafe audit details: %s", event.DetailsJSON)
	}
}
