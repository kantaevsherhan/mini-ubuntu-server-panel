package notifications

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/kantaevsherhan/mini-ubuntu-server-panel/backend/internal/database"
)

type fakeSender struct {
	messages []string
	err      error
}

func (f *fakeSender) Send(_ context.Context, _ int64, message string) error {
	f.messages = append(f.messages, message)
	return f.err
}

func TestEnqueueDeduplicatesAndDelivers(t *testing.T) {
	db, err := database.Open(filepath.Join(t.TempDir(), "panel.db"))
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	defer func() { _ = sqlDB.Close() }()
	name := "Admin"
	err = db.Create(&database.TelegramRecipient{TelegramChatID: 42, DisplayName: &name, Enabled: true, ReceiveAlerts: true, CreatedAt: time.Now().UTC()}).Error
	if err != nil {
		t.Fatal(err)
	}
	sender := &fakeSender{}
	service := New(db, sender)
	event := Event{Key: "cpu.high", Severity: "warning", DedupKey: "cpu.high:1", Payload: map[string]any{"title": "High CPU", "message": "CPU 95%"}}
	firstID, err := service.Enqueue(context.Background(), event)
	if err != nil {
		t.Fatal(err)
	}
	secondID, err := service.Enqueue(context.Background(), event)
	if err != nil || secondID != firstID {
		t.Fatalf("dedup failed: %d %d %v", firstID, secondID, err)
	}
	processed, err := service.ProcessOnce(context.Background())
	if err != nil || !processed || len(sender.messages) != 1 {
		t.Fatalf("delivery failed: %v %v %#v", processed, err, sender.messages)
	}
	var delivery database.NotificationDelivery
	if err := db.Where("event_id = ?", firstID).First(&delivery).Error; err != nil || delivery.Status != "delivered" {
		t.Fatalf("unexpected delivery status %q: %v", delivery.Status, err)
	}
}

func TestDeliveryRetriesWithBackoff(t *testing.T) {
	db, err := database.Open(filepath.Join(t.TempDir(), "panel.db"))
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	defer func() { _ = sqlDB.Close() }()
	_ = db.Model(&database.TelegramSetting{}).Where("id = ?", 1).Update("retry_count", 1).Error
	name := "Admin"
	_ = db.Create(&database.TelegramRecipient{TelegramChatID: 42, DisplayName: &name, Enabled: true, ReceiveAlerts: true, CreatedAt: time.Now().UTC()}).Error
	sender := &fakeSender{err: errors.New("token redacted failure")}
	service := New(db, sender)
	service.now = func() time.Time { return time.Date(2026, 7, 23, 12, 0, 0, 0, time.UTC) }
	eventID, err := service.Enqueue(context.Background(), Event{Key: "disk.full", Severity: "critical", DedupKey: "disk.full:1", Payload: map[string]any{"message": "Disk full"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.ProcessOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	var delivery database.NotificationDelivery
	if err := db.Where("event_id = ?", eventID).First(&delivery).Error; err != nil {
		t.Fatal(err)
	}
	if delivery.Status != "pending" || delivery.Attempts != 1 || delivery.NextAttemptAt == nil || !delivery.NextAttemptAt.After(service.now()) {
		t.Fatalf("unexpected retry state: %#v", delivery)
	}
}
