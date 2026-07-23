package httpapi

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/kantaevsherhan/mini-ubuntu-server-panel/backend/internal/auth"
	"github.com/kantaevsherhan/mini-ubuntu-server-panel/backend/internal/database"
	"github.com/kantaevsherhan/mini-ubuntu-server-panel/backend/internal/services"
)

func (a API) serviceList(c *fiber.Ctx) error {
	if a.Services == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"error": "service_manager_unavailable"})
	}
	ctx, cancel := context.WithTimeout(c.UserContext(), 10*time.Second)
	defer cancel()
	items, err := a.Services.List(ctx)
	if err != nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"error": "service_list_failed"})
	}
	return c.JSON(items)
}

func (a API) serviceAction(c *fiber.Ctx) error {
	if a.Services == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"error": "service_manager_unavailable"})
	}
	unit := strings.TrimSpace(c.Params("unit"))
	var request struct {
		Action string `json:"action"`
	}
	if err := c.BodyParser(&request); err != nil {
		return fiber.ErrBadRequest
	}
	request.Action = strings.ToLower(strings.TrimSpace(request.Action))
	if err := services.ValidateAction(unit, request.Action); err != nil {
		return c.Status(fiber.StatusUnprocessableEntity).JSON(fiber.Map{"error": "service_action_invalid"})
	}
	ctx, cancel := context.WithTimeout(c.UserContext(), 30*time.Second)
	defer cancel()
	if err := a.Services.Action(ctx, unit, request.Action); err != nil {
		return c.Status(fiber.StatusConflict).JSON(fiber.Map{"error": "service_action_failed"})
	}
	claims := c.Locals("claims").(*auth.Claims)
	database.Audit(a.DB, claims.UserID, "service."+request.Action, "systemd_service", unit, `{"action":`+strconv.Quote(request.Action)+`}`, c.IP())
	return c.SendStatus(fiber.StatusNoContent)
}
