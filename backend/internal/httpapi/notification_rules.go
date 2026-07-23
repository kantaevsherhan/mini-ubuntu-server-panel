package httpapi

import (
	"errors"
	"regexp"
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/kantaevsherhan/mini-ubuntu-server-panel/backend/internal/auth"
	"github.com/kantaevsherhan/mini-ubuntu-server-panel/backend/internal/database"
	"gorm.io/gorm"
)

var eventKeyPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_.-]{1,127}$`)

type notificationRuleRequest struct {
	Enabled               bool    `json:"enabled"`
	Severity              string  `json:"severity"`
	RecipientIDs          []int64 `json:"recipient_ids"`
	CooldownSeconds       int     `json:"cooldown_seconds"`
	RepeatIntervalSeconds int     `json:"repeat_interval_seconds"`
	SendRecovery          bool    `json:"send_recovery"`
}

func (a API) notificationRules(c *fiber.Ctx) error {
	var rules []database.NotificationRule
	if err := a.DB.WithContext(c.UserContext()).Order("event_key").Find(&rules).Error; err != nil {
		return err
	}
	var links []database.NotificationRuleRecipient
	if err := a.DB.WithContext(c.UserContext()).Find(&links).Error; err != nil {
		return err
	}
	recipients := make(map[string][]int64)
	for _, link := range links {
		recipients[link.EventKey] = append(recipients[link.EventKey], link.RecipientID)
	}
	result := make([]fiber.Map, 0, len(rules))
	for _, rule := range rules {
		result = append(result, fiber.Map{
			"event_key": rule.EventKey, "enabled": rule.Enabled, "severity": rule.Severity,
			"recipient_ids": recipients[rule.EventKey], "cooldown_seconds": rule.CooldownSeconds,
			"repeat_interval_seconds": rule.RepeatIntervalSeconds, "send_recovery": rule.SendRecovery,
			"updated_at": rule.UpdatedAt,
		})
	}
	return c.JSON(result)
}

func (a API) updateNotificationRule(c *fiber.Ctx) error {
	eventKey := c.Params("key")
	var request notificationRuleRequest
	if !eventKeyPattern.MatchString(eventKey) || c.BodyParser(&request) != nil || !validSeverity(request.Severity) || request.CooldownSeconds < 0 || request.CooldownSeconds > 7*24*3600 || request.RepeatIntervalSeconds < 0 || request.RepeatIntervalSeconds > 30*24*3600 || len(request.RecipientIDs) > 100 {
		return c.Status(fiber.StatusUnprocessableEntity).JSON(fiber.Map{"error": "notification_rule_invalid"})
	}
	request.RecipientIDs = uniquePositiveIDs(request.RecipientIDs)
	if request.RecipientIDs == nil {
		return c.Status(fiber.StatusUnprocessableEntity).JSON(fiber.Map{"error": "notification_recipients_invalid"})
	}
	err := a.DB.WithContext(c.UserContext()).Transaction(func(tx *gorm.DB) error {
		var rule database.NotificationRule
		if err := tx.First(&rule, "event_key = ?", eventKey).Error; err != nil {
			return err
		}
		if len(request.RecipientIDs) > 0 {
			var count int64
			if err := tx.Model(&database.TelegramRecipient{}).Where("id IN ?", request.RecipientIDs).Count(&count).Error; err != nil {
				return err
			}
			if count != int64(len(request.RecipientIDs)) {
				return errors.New("unknown notification recipient")
			}
		}
		if err := tx.Model(&rule).Updates(map[string]any{
			"enabled": request.Enabled, "severity": request.Severity, "cooldown_seconds": request.CooldownSeconds,
			"repeat_interval_seconds": request.RepeatIntervalSeconds, "send_recovery": request.SendRecovery, "updated_at": time.Now().UTC(),
		}).Error; err != nil {
			return err
		}
		if err := tx.Where("event_key = ?", eventKey).Delete(&database.NotificationRuleRecipient{}).Error; err != nil {
			return err
		}
		links := make([]database.NotificationRuleRecipient, 0, len(request.RecipientIDs))
		for _, recipientID := range request.RecipientIDs {
			links = append(links, database.NotificationRuleRecipient{EventKey: eventKey, RecipientID: recipientID})
		}
		if len(links) > 0 {
			return tx.Create(&links).Error
		}
		return nil
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return fiber.ErrNotFound
	}
	if err != nil {
		return c.Status(fiber.StatusUnprocessableEntity).JSON(fiber.Map{"error": "notification_rule_update_failed"})
	}
	claims := c.Locals("claims").(*auth.Claims)
	database.Audit(a.DB, claims.UserID, "notification.rule.update", "notification_rule", eventKey, `{"recipients":"configured"}`, c.IP())
	return c.SendStatus(fiber.StatusNoContent)
}

func (a API) notificationHistory(c *fiber.Ctx) error {
	limit, _ := strconv.Atoi(c.Query("limit", "200"))
	if limit < 1 || limit > 500 {
		return c.Status(fiber.StatusUnprocessableEntity).JSON(fiber.Map{"error": "notification_history_limit_invalid"})
	}
	var events []database.NotificationEvent
	if err := a.DB.WithContext(c.UserContext()).Order("id DESC").Limit(limit).Find(&events).Error; err != nil {
		return err
	}
	ids := make([]int64, 0, len(events))
	for _, event := range events {
		ids = append(ids, event.ID)
	}
	var deliveries []database.NotificationDelivery
	if len(ids) > 0 {
		if err := a.DB.WithContext(c.UserContext()).Where("event_id IN ?", ids).Order("id").Find(&deliveries).Error; err != nil {
			return err
		}
	}
	byEvent := make(map[int64][]fiber.Map)
	for _, delivery := range deliveries {
		lastError := ""
		if delivery.LastError != nil {
			lastError = "delivery_failed"
		}
		byEvent[delivery.EventID] = append(byEvent[delivery.EventID], fiber.Map{
			"id": delivery.ID, "recipient_id": delivery.RecipientID, "status": delivery.Status,
			"attempts": delivery.Attempts, "last_error": lastError, "next_attempt_at": delivery.NextAttemptAt,
			"delivered_at": delivery.DeliveredAt,
		})
	}
	result := make([]fiber.Map, 0, len(events))
	for _, event := range events {
		result = append(result, fiber.Map{"id": event.ID, "event_key": event.EventKey, "severity": event.Severity, "status": event.Status, "created_at": event.CreatedAt, "resolved_at": event.ResolvedAt, "deliveries": byEvent[event.ID]})
	}
	return c.JSON(result)
}

func validSeverity(value string) bool {
	switch value {
	case "info", "warning", "error", "critical":
		return true
	default:
		return false
	}
}

func uniquePositiveIDs(values []int64) []int64 {
	result := make([]int64, 0, len(values))
	seen := make(map[int64]bool, len(values))
	for _, value := range values {
		if value <= 0 {
			return nil
		}
		if !seen[value] {
			seen[value] = true
			result = append(result, value)
		}
	}
	return result
}
