package httpapi

import (
	"context"
	"errors"
	"github.com/gofiber/fiber/v2"
	"github.com/kantaevsherhan/mini-ubuntu-server-panel/backend/internal/database"
	"time"
)

var databaseSlots = make(chan struct{}, 4)

func (a API) databaseBrowse(c *fiber.Ctx) error {
	c.Set("Cache-Control", "no-store")
	var request database.ExternalRequest
	if len(c.Body()) > 16384 || c.BodyParser(&request) != nil {
		return fiber.ErrBadRequest
	}
	if err := database.ValidateExternal(request); err != nil {
		return c.Status(422).JSON(fiber.Map{"error": "database_request_invalid"})
	}
	select {
	case databaseSlots <- struct{}{}:
		defer func() { <-databaseSlots }()
	default:
		return c.Status(429).JSON(fiber.Map{"error": "database_busy"})
	}
	ctx, cancel := context.WithTimeout(c.UserContext(), 12*time.Second)
	defer cancel()
	result, err := database.BrowseExternal(ctx, request)
	// Record only engine and outcome, never connection credentials or returned data.
	outcome := "success"
	if err != nil {
		outcome = "failed"
	}
	a.dockerAudit(c, "database.browse", "external_database", request.Engine, `{"outcome":"`+outcome+`"}`)
	if errors.Is(err, database.ErrExternalInvalid) {
		return c.Status(422).JSON(fiber.Map{"error": "database_request_invalid"})
	}
	if errors.Is(err, database.ErrExternalTooLarge) {
		return c.Status(413).JSON(fiber.Map{"error": "database_result_too_large"})
	}
	if err != nil {
		return c.Status(502).JSON(fiber.Map{"error": "database_connection_failed"})
	}
	return c.JSON(result)
}
