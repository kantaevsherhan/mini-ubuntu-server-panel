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
	Recovery bool           `json:"recovery"`
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
	var model database.NotificationEvent
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		defaultRule := database.NotificationRule{EventKey: event.Key, Enabled: true, Severity: event.Severity, CooldownSeconds: 600, RepeatIntervalSeconds: 1800, SendRecovery: true, UpdatedAt: now}
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&defaultRule).Error; err != nil {
			return err
		}
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&database.NotificationRuleState{EventKey: event.Key}).Error; err != nil {
			return err
		}
		var rule database.NotificationRule
		var state database.NotificationRuleState
		if err := tx.First(&rule, "event_key = ?", event.Key).Error; err != nil {
			return err
		}
		if err := tx.First(&state, "event_key = ?", event.Key).Error; err != nil {
			return err
		}
		if !rule.Enabled {
			return nil
		}
		if event.Recovery {
			return s.enqueueRecovery(tx, event, payload, rule, state, now, &model)
		}
		if state.Active && state.LastNotifiedAt != nil {
			if rule.RepeatIntervalSeconds <= 0 || now.Before(state.LastNotifiedAt.Add(time.Duration(rule.RepeatIntervalSeconds)*time.Second)) {
				if state.LastEventID != nil {
					model.ID = *state.LastEventID
				}
				return nil
			}
		} else if !state.Active && state.ResolvedAt != nil && rule.CooldownSeconds > 0 && now.Before(state.ResolvedAt.Add(time.Duration(rule.CooldownSeconds)*time.Second)) {
			if state.LastEventID != nil {
				model.ID = *state.LastEventID
			}
			return nil
		}
		dedupKey := event.DedupKey
		if state.Active {
			dedupKey = fmt.Sprintf("%s:repeat:%d", event.DedupKey, now.UnixNano())
		}
		model = database.NotificationEvent{EventKey: event.Key, Severity: rule.Severity, PayloadJSON: string(payload), DedupKey: &dedupKey, Status: "pending", CreatedAt: now, UpdatedAt: &now}
		result := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&model)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return tx.Where("dedup_key = ?", dedupKey).First(&model).Error
		}
		if err := s.createDeliveries(tx, event.Audience, rule.EventKey, model.ID, now); err != nil {
			return err
		}
		updates := map[string]any{"active": true, "active_dedup_key": event.DedupKey, "last_event_id": model.ID, "last_notified_at": now, "resolved_at": nil}
		if !state.Active {
			updates["last_triggered_at"] = now
		}
		return tx.Model(&state).Updates(updates).Error
	})
	return model.ID, err
}

func (s *Service) enqueueRecovery(tx *gorm.DB, event Event, payload []byte, rule database.NotificationRule, state database.NotificationRuleState, now time.Time, model *database.NotificationEvent) error {
	if !state.Active {
		return nil
	}
	if err := tx.Model(&database.NotificationEvent{}).Where("event_key = ? AND resolved_at IS NULL", event.Key).Updates(map[string]any{"status": "resolved", "resolved_at": now, "updated_at": now}).Error; err != nil {
		return err
	}
	if err := tx.Model(&database.NotificationDelivery{}).Where("status = ? AND event_id IN (?)", "pending", tx.Model(&database.NotificationEvent{}).Select("id").Where("event_key = ? AND resolved_at = ?", event.Key, now)).Update("status", "cancelled").Error; err != nil {
		return err
	}
	stateUpdates := map[string]any{"active": false, "active_dedup_key": nil, "resolved_at": now}
	if !rule.SendRecovery {
		return tx.Model(&state).Updates(stateUpdates).Error
	}
	dedupKey := fmt.Sprintf("%s:recovery:%d", event.DedupKey, now.UnixNano())
	*model = database.NotificationEvent{EventKey: event.Key, Severity: "recovery", PayloadJSON: string(payload), DedupKey: &dedupKey, Status: "pending", CreatedAt: now, UpdatedAt: &now, ResolvedAt: &now}
	if err := tx.Create(model).Error; err != nil {
		return err
	}
	if err := s.createDeliveries(tx, event.Audience, rule.EventKey, model.ID, now); err != nil {
		return err
	}
	stateUpdates["last_event_id"] = model.ID
	stateUpdates["last_notified_at"] = now
	return tx.Model(&state).Updates(stateUpdates).Error
}

func (s *Service) createDeliveries(tx *gorm.DB, audience, eventKey string, eventID int64, now time.Time) error {
	var selectedCount int64
	if err := tx.Model(&database.NotificationRuleRecipient{}).Where("event_key = ?", eventKey).Count(&selectedCount).Error; err != nil {
		return err
	}
	query := tx.Model(&database.TelegramRecipient{}).Where("enabled = ?", true)
	if selectedCount > 0 {
		query = query.Joins("JOIN notification_rule_recipients ON notification_rule_recipients.recipient_id = telegram_recipients.id").Where("notification_rule_recipients.event_key = ?", eventKey)
	} else {
		switch audience {
		case "audit":
			query = query.Where("receive_audit = ?", true)
		case "updates":
			query = query.Where("receive_updates = ?", true)
		default:
			query = query.Where("receive_alerts = ?", true)
		}
	}
	var recipients []database.TelegramRecipient
	if err := query.Find(&recipients).Error; err != nil {
		return err
	}
	if len(recipients) == 0 {
		return tx.Model(&database.NotificationEvent{}).Where("id = ?", eventID).Updates(map[string]any{"status": "skipped", "updated_at": now}).Error
	}
	deliveries := make([]database.NotificationDelivery, 0, len(recipients))
	for _, recipient := range recipients {
		deliveries = append(deliveries, database.NotificationDelivery{EventID: eventID, RecipientID: recipient.ID, Status: "pending", NextAttemptAt: &now, CreatedAt: now})
	}
	return tx.Create(&deliveries).Error
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
		if updateErr := s.db.WithContext(ctx).Model(&delivery).Updates(map[string]any{"status": delivery.Status, "attempts": delivery.Attempts, "last_error": message, "next_attempt_at": next}).Error; updateErr != nil {
			return true, updateErr
		}
		return true, s.updateEventStatus(ctx, event.ID, now)
	}
	delivered := now
	delivery.Attempts++
	if err = s.db.WithContext(ctx).Model(&delivery).Updates(map[string]any{"status": "delivered", "attempts": delivery.Attempts, "last_error": nil, "delivered_at": delivered}).Error; err != nil {
		return true, err
	}
	return true, s.updateEventStatus(ctx, event.ID, now)
}

func (s *Service) updateEventStatus(ctx context.Context, eventID int64, now time.Time) error {
	var pending, failed int64
	if err := s.db.WithContext(ctx).Model(&database.NotificationDelivery{}).Where("event_id = ? AND status IN ?", eventID, []string{"pending", "sending"}).Count(&pending).Error; err != nil {
		return err
	}
	if err := s.db.WithContext(ctx).Model(&database.NotificationDelivery{}).Where("event_id = ? AND status = ?", eventID, "failed").Count(&failed).Error; err != nil {
		return err
	}
	status := "delivered"
	if pending > 0 {
		status = "pending"
	} else if failed > 0 {
		status = "failed"
	}
	return s.db.WithContext(ctx).Model(&database.NotificationEvent{}).Where("id = ? AND status <> ?", eventID, "resolved").Updates(map[string]any{"status": status, "updated_at": now}).Error
}

func (s *Service) RecoverInFlight(ctx context.Context) error {
	now := s.now()
	return s.db.WithContext(ctx).Model(&database.NotificationDelivery{}).Where("status = ?", "sending").Updates(map[string]any{"status": "pending", "next_attempt_at": now}).Error
}

func (s *Service) Run(ctx context.Context) {
	_ = s.RecoverInFlight(ctx)
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
