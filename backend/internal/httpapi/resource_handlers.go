package httpapi

import (
	"context"
	"encoding/json"
	"github.com/gofiber/fiber/v2"
	dm "github.com/kantaevsherhan/mini-ubuntu-server-panel/backend/internal/docker"
	"time"
)

type dockerResources interface {
	Stats(context.Context) (map[string]dm.ContainerStats, error)
	UpdateLimits(context.Context, string, dm.ResourceLimits) error
}

func (a API) dockerStats(c *fiber.Ctx) error {
	controller, ok := a.Docker.(dockerResources)
	if !ok {
		return a.dockerUnavailable(c)
	}
	ctx, cancel := context.WithTimeout(c.UserContext(), 12*time.Second)
	defer cancel()
	items, err := controller.Stats(ctx)
	if err != nil {
		return c.Status(503).JSON(fiber.Map{"error": "docker_stats_failed"})
	}
	return c.JSON(items)
}

func (a API) dockerLimits(c *fiber.Ctx) error {
	controller, ok := a.Docker.(dockerResources)
	if !ok {
		return a.dockerUnavailable(c)
	}
	var l dm.ResourceLimits
	if err := c.BodyParser(&l); err != nil {
		return fiber.ErrBadRequest
	}
	ctx, cancel := context.WithTimeout(c.UserContext(), 10*time.Second)
	defer cancel()
	if err := controller.UpdateLimits(ctx, c.Params("id"), l); err != nil {
		return dockerError(c, err, "docker_limits_failed")
	}
	details, _ := json.Marshal(l)
	a.dockerAudit(c, "docker.container.limits", "docker_container", c.Params("id"), string(details))
	return c.SendStatus(fiber.StatusNoContent)
}
