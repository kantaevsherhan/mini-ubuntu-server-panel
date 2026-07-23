package httpapi

import (
	"github.com/gofiber/fiber/v2"
	"github.com/kantaevsherhan/mini-ubuntu-server-panel/backend/internal/database"
)

func (a API) audit(c *fiber.Ctx) error {
	var events []database.AuditEvent
	err := a.DB.WithContext(c.UserContext()).Order("id DESC").Limit(200).Find(&events).Error
	if err != nil {
		return err
	}
	result := make([]fiber.Map, 0)
	for _, event := range events {
		result = append(result, fiber.Map{
			"id": event.ID, "actor_user_id": event.ActorUserID, "action": event.Action,
			"target_type": event.TargetType, "target_id": event.TargetID, "details": event.DetailsJSON,
			"ip_address": event.IPAddress, "created_at": event.CreatedAt,
		})
	}
	return c.JSON(result)
}
