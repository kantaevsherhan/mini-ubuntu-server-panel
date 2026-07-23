package httpapi

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/kantaevsherhan/mini-ubuntu-server-panel/backend/internal/auth"
	"github.com/kantaevsherhan/mini-ubuntu-server-panel/backend/internal/database"
	dockermanager "github.com/kantaevsherhan/mini-ubuntu-server-panel/backend/internal/docker"
)

func (a API) dockerContainers(c *fiber.Ctx) error {
	if a.Docker == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"error": "docker_service_unavailable"})
	}
	ctx, cancel := context.WithTimeout(c.UserContext(), 10*time.Second)
	defer cancel()
	items, err := a.Docker.List(ctx)
	if err != nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"error": "docker_unavailable"})
	}
	return c.JSON(items)
}

func (a API) dockerContainerAction(c *fiber.Ctx) error {
	if a.Docker == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"error": "docker_service_unavailable"})
	}
	id := strings.ToLower(strings.TrimSpace(c.Params("id")))
	var request struct {
		Action string `json:"action"`
	}
	if err := c.BodyParser(&request); err != nil {
		return fiber.ErrBadRequest
	}
	request.Action = strings.ToLower(strings.TrimSpace(request.Action))
	if err := dockermanager.ValidateAction(id, request.Action); err != nil {
		return c.Status(fiber.StatusUnprocessableEntity).JSON(fiber.Map{"error": "docker_action_invalid"})
	}
	ctx, cancel := context.WithTimeout(c.UserContext(), 45*time.Second)
	defer cancel()
	if err := a.Docker.Action(ctx, id, request.Action); err != nil {
		return c.Status(fiber.StatusConflict).JSON(fiber.Map{"error": "docker_action_failed"})
	}
	claims := c.Locals("claims").(*auth.Claims)
	database.Audit(a.DB, claims.UserID, "docker.container."+request.Action, "docker_container", id, `{"action":`+strconv.Quote(request.Action)+`}`, c.IP())
	return c.SendStatus(fiber.StatusNoContent)
}
