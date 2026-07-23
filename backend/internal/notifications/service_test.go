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

func TestRuleCooldownRepeatAndRecovery(t *testing.T) {
	db, err := database.Open(filepath.Join(t.TempDir(), "panel.db"))
	if err != nil {
		t.Fatal(err)
	}
	name := "Admin"
	if err := db.Create(&database.TelegramRecipient{TelegramChatID: 42, DisplayName: &name, Enabled: true, ReceiveAlerts: true, CreatedAt: time.Now().UTC()}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&database.NotificationRule{}).Where("event_key = ?", "resource.cpu.high").Updates(map[string]any{"cooldown_seconds": 120, "repeat_interval_seconds": 60, "send_recovery": true}).Error; err != nil {
		t.Fatal(err)
	}
	service := New(db, &fakeSender{})
	current := time.Date(2026, 7, 23, 12, 0, 0, 0, time.UTC)
	service.now = func() time.Time { return current }
	event := Event{Key: "resource.cpu.high", Severity: "warning", DedupKey: "cpu:server", Payload: map[string]any{"message": "CPU 95%"}}
	firstID, err := service.Enqueue(context.Background(), event)
	if err != nil || firstID == 0 {
		t.Fatalf("initial enqueue failed: id=%d err=%v", firstID, err)
	}
	current = current.Add(30 * time.Second)
	suppressedID, _ := service.Enqueue(context.Background(), event)
	if suppressedID != firstID {
		t.Fatalf("repeat was not suppressed: %d != %d", suppressedID, firstID)
	}
	current = current.Add(31 * time.Second)
	repeatID, _ := service.Enqueue(context.Background(), event)
	if repeatID == 0 || repeatID == firstID {
		t.Fatalf("repeat was not emitted: %d", repeatID)
	}
	current = current.Add(9 * time.Second)
	event.Recovery = true
	event.Payload = map[string]any{"message": "CPU normal"}
	recoveryID, err := service.Enqueue(context.Background(), event)
	if err != nil || recoveryID == 0 {
		t.Fatalf("recovery enqueue failed: id=%d err=%v", recoveryID, err)
	}
	var recovery database.NotificationEvent
	if err := db.First(&recovery, recoveryID).Error; err != nil || recovery.Severity != "recovery" {
		t.Fatalf("unexpected recovery: %#v err=%v", recovery, err)
	}
	current = current.Add(30 * time.Second)
	event.Recovery = false
	cooldownID, _ := service.Enqueue(context.Background(), event)
	if cooldownID != recoveryID {
		t.Fatalf("cooldown did not suppress event: %d != %d", cooldownID, recoveryID)
	}
	current = current.Add(121 * time.Second)
	afterCooldownID, _ := service.Enqueue(context.Background(), event)
	if afterCooldownID == 0 || afterCooldownID == recoveryID {
		t.Fatalf("event after cooldown was not emitted: %d", afterCooldownID)
	}
}

func TestRecoverInFlightDeliveries(t *testing.T) {
	db, err := database.Open(filepath.Join(t.TempDir(), "panel.db"))
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	recipient := database.TelegramRecipient{TelegramChatID: 77, Enabled: true, ReceiveAlerts: true, CreatedAt: now}
	if err := db.Create(&recipient).Error; err != nil {
		t.Fatal(err)
	}
	event := database.NotificationEvent{EventKey: "test", Severity: "warning", PayloadJSON: `{}`, Status: "pending", CreatedAt: now}
	if err := db.Create(&event).Error; err != nil {
		t.Fatal(err)
	}
	delivery := database.NotificationDelivery{EventID: event.ID, RecipientID: recipient.ID, Status: "sending", CreatedAt: now}
	if err := db.Create(&delivery).Error; err != nil {
		t.Fatal(err)
	}
	service := New(db, &fakeSender{})
	if err := service.RecoverInFlight(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := db.First(&delivery, delivery.ID).Error; err != nil || delivery.Status != "pending" || delivery.NextAttemptAt == nil {
		t.Fatalf("delivery was not recovered: %#v err=%v", delivery, err)
	}
}
