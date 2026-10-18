package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/kantaevsherhan/mini-ubuntu-server-panel/backend/internal/aptupdates"
	"github.com/kantaevsherhan/mini-ubuntu-server-panel/backend/internal/auth"
	"github.com/kantaevsherhan/mini-ubuntu-server-panel/backend/internal/database"
)

func (a API) aptStatus(c *fiber.Ctx) error {
	if a.Apt == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"error": "apt_unavailable"})
	}
	ctx, cancel := context.WithTimeout(c.UserContext(), 90*time.Second)
	defer cancel()
	status, err := a.Apt.Status(ctx)
	if err != nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"error": "apt_unavailable"})
	}
	return c.JSON(status)
}

func (a API) aptJob(c *fiber.Ctx) error {
	if a.Apt == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"error": "apt_unavailable"})
	}
	ctx, cancel := context.WithTimeout(c.UserContext(), 15*time.Second)
	defer cancel()
	job, err := a.Apt.Job(ctx)
	if err != nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"error": "apt_unavailable"})
	}
	return c.JSON(job)
}

func (a API) aptStart(operation string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		if a.Apt == nil {
			return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"error": "apt_unavailable"})
		}
		request := aptupdates.StartRequest{Operation: operation}
		if operation == "upgrade" && len(c.Body()) > 0 {
			var body struct {
				Packages []string `json:"packages"`
			}
			if err := c.BodyParser(&body); err != nil {
				return fiber.ErrBadRequest
			}
			request.Packages = body.Packages
		}
		ctx, cancel := context.WithTimeout(c.UserContext(), 2*time.Minute)
		defer cancel()
		err := a.Apt.Start(ctx, request)
		switch {
		case errors.Is(err, aptupdates.ErrInvalidRequest):
			return c.Status(fiber.StatusUnprocessableEntity).JSON(fiber.Map{"error": "apt_request_invalid"})
		case errors.Is(err, aptupdates.ErrBusy):
			return c.Status(fiber.StatusConflict).JSON(fiber.Map{"error": "apt_job_running"})
		case err != nil:
			return c.Status(fiber.StatusConflict).JSON(fiber.Map{"error": "apt_job_start_failed"})
		}
		claims := c.Locals("claims").(*auth.Claims)
		details, _ := json.Marshal(fiber.Map{"operation": operation, "packages": request.Packages, "all": operation == "upgrade" && len(request.Packages) == 0})
		database.Audit(a.DB, claims.UserID, "apt."+operation, "apt", aptupdates.UnitName, string(details), c.IP())
		return c.SendStatus(fiber.StatusAccepted)
	}
}
