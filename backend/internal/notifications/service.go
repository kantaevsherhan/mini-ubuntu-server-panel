package notifications

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/kantaevsherhan/mini-ubuntu-server-panel/backend/internal/database"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type Event struct {
	Key      string         `json:"key"`
	Severity string         `json:"severity"`
	Payload  map[string]any `json:"payload"`
	DedupKey string         `json:"dedup_key"`
	Audience string         `json:"audience"`
}
type Sender interface {
	Send(context.Context, int64, string) error
}
type Service struct {
	db     *gorm.DB
	sender Sender
	now    func() time.Time
}

func New(db *gorm.DB, sender Sender) *Service {
	return &Service{db: db, sender: sender, now: func() time.Time { return time.Now().UTC() }}
}

func (s *Service) Enqueue(ctx context.Context, event Event) (int64, error) {
	if event.Key == "" || event.Severity == "" || event.DedupKey == "" {
		return 0, errors.New("invalid notification event")
	}
	payload, err := json.Marshal(event.Payload)
	if err != nil {
		return 0, err
	}
	now := s.now()
	model := database.NotificationEvent{EventKey: event.Key, Severity: event.Severity, PayloadJSON: string(payload), DedupKey: &event.DedupKey, Status: "pending", CreatedAt: now, UpdatedAt: &now}
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&model)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return tx.Where("dedup_key = ?", event.DedupKey).First(&model).Error
		}
		query := tx.Where("enabled = ?", true)
		switch event.Audience {
		case "audit":
			query = query.Where("receive_audit = ?", true)
		case "updates":
			query = query.Where("receive_updates = ?", true)
		default:
			query = query.Where("receive_alerts = ?", true)
		}
		var recipients []database.TelegramRecipient
		if err := query.Find(&recipients).Error; err != nil {
			return err
		}
		deliveries := make([]database.NotificationDelivery, 0, len(recipients))
		for _, recipient := range recipients {
			deliveries = append(deliveries, database.NotificationDelivery{EventID: model.ID, RecipientID: recipient.ID, Status: "pending", NextAttemptAt: &now, CreatedAt: now})
		}
		if len(deliveries) > 0 {
			return tx.Create(&deliveries).Error
		}
		return nil
	})
	return model.ID, err
}

func (s *Service) ProcessOnce(ctx context.Context) (bool, error) {
	now := s.now()
	var delivery database.NotificationDelivery
	err := s.db.WithContext(ctx).Where("status = ? AND (next_attempt_at IS NULL OR next_attempt_at <= ?)", "pending", now).Order("id").First(&delivery).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	claimed := s.db.WithContext(ctx).Model(&database.NotificationDelivery{}).Where("id = ? AND status = ?", delivery.ID, "pending").Update("status", "sending")
	if claimed.Error != nil {
		return false, claimed.Error
	}
	if claimed.RowsAffected == 0 {
		return true, nil
	}
	var event database.NotificationEvent
	var recipient database.TelegramRecipient
	var settings database.TelegramSetting
	if err = s.db.WithContext(ctx).First(&event, delivery.EventID).Error; err != nil {
		return true, err
	}
	if err = s.db.WithContext(ctx).First(&recipient, delivery.RecipientID).Error; err != nil {
		return true, err
	}
	if err = s.db.WithContext(ctx).First(&settings, 1).Error; err != nil {
		return true, err
	}
	if err = s.sender.Send(ctx, recipient.TelegramChatID, formatMessage(event.EventKey, event.Severity, event.PayloadJSON)); err != nil {
		delivery.Attempts++
		delivery.Status = "pending"
		if delivery.Attempts > settings.RetryCount {
			delivery.Status = "failed"
		}
		message := truncate(err.Error(), 500)
		next := now.Add(time.Duration(math.Min(math.Pow(2, float64(delivery.Attempts)), 3600)) * time.Second)
		return true, s.db.WithContext(ctx).Model(&delivery).Updates(map[string]any{"status": delivery.Status, "attempts": delivery.Attempts, "last_error": message, "next_attempt_at": next}).Error
	}
	delivered := now
	delivery.Attempts++
	if err = s.db.WithContext(ctx).Model(&delivery).Updates(map[string]any{"status": "delivered", "attempts": delivery.Attempts, "last_error": nil, "delivered_at": delivered}).Error; err != nil {
		return true, err
	}
	var pending int64
	if err = s.db.WithContext(ctx).Model(&database.NotificationDelivery{}).Where("event_id = ? AND status IN ?", event.ID, []string{"pending", "sending"}).Count(&pending).Error; err != nil {
		return true, err
	}
	status := "delivered"
	if pending > 0 {
		status = "pending"
	}
	return true, s.db.WithContext(ctx).Model(&event).Updates(map[string]any{"status": status, "updated_at": now}).Error
}

func (s *Service) Run(ctx context.Context) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			for processed := true; processed; {
				processed, _ = s.ProcessOnce(ctx)
			}
		}
	}
}
func formatMessage(key, severity, payloadJSON string) string {
	var payload map[string]any
	_ = json.Unmarshal([]byte(payloadJSON), &payload)
	title, _ := payload["title"].(string)
	body, _ := payload["message"].(string)
	if title == "" {
		title = key
	}
	return fmt.Sprintf("%s %s\n\n%s", severityIcon(severity), title, body)
}
func severityIcon(severity string) string {
	switch severity {
	case "critical", "error":
		return "🔴"
	case "warning":
		return "🟠"
	case "success", "recovery":
		return "🟢"
	default:
		return "🔵"
	}
}
func truncate(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	return value[:limit]
}
