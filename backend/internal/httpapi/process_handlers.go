package httpapi

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/kantaevsherhan/mini-ubuntu-server-panel/backend/internal/auth"
	"github.com/kantaevsherhan/mini-ubuntu-server-panel/backend/internal/database"
)

func (a API) processList(c *fiber.Ctx) error {
	if a.Processes == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"error": "process_service_unavailable"})
	}
	ctx, cancel := context.WithTimeout(c.UserContext(), 5*time.Second)
	defer cancel()
	items, err := a.Processes.List(ctx)
	if err != nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"error": "process_list_failed"})
	}
	return c.JSON(items)
}

func (a API) processSignal(c *fiber.Ctx) error {
	if a.Processes == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"error": "process_service_unavailable"})
	}
	value, err := strconv.ParseInt(c.Params("pid"), 10, 32)
	if err != nil || value <= 1 {
		return c.Status(fiber.StatusUnprocessableEntity).JSON(fiber.Map{"error": "process_pid_invalid"})
	}
	var request struct {
		Signal string `json:"signal"`
	}
	if err := c.BodyParser(&request); err != nil {
		return fiber.ErrBadRequest
	}
	request.Signal = strings.ToUpper(strings.TrimSpace(request.Signal))
	if request.Signal != "TERM" && request.Signal != "KILL" && request.Signal != "HUP" {
		return c.Status(fiber.StatusUnprocessableEntity).JSON(fiber.Map{"error": "process_signal_invalid"})
	}
	ctx, cancel := context.WithTimeout(c.UserContext(), 5*time.Second)
	defer cancel()
	if err := a.Processes.Signal(ctx, int(value), request.Signal); err != nil {
		return c.Status(fiber.StatusConflict).JSON(fiber.Map{"error": "process_signal_failed"})
	}
	claims := c.Locals("claims").(*auth.Claims)
	database.Audit(a.DB, claims.UserID, "process.signal", "process", strconv.FormatInt(value, 10), `{"signal":`+strconv.Quote(request.Signal)+`}`, c.IP())
	return c.SendStatus(fiber.StatusNoContent)
}
