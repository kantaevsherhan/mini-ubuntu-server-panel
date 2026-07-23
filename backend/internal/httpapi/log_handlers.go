package httpapi

import (
	"context"
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/kantaevsherhan/mini-ubuntu-server-panel/backend/internal/logs"
)

func (a API) logsList(c *fiber.Ctx) error {
	if a.Logs == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"error": "logs_service_unavailable"})
	}
	limit := 1000
	if value := c.Query("limit"); value != "" {
		parsed, err := strconv.Atoi(value)
		if err != nil {
			return c.Status(fiber.StatusUnprocessableEntity).JSON(fiber.Map{"error": "logs_query_invalid"})
		}
		limit = parsed
	}
	query := logs.Normalize(logs.Query{Unit: c.Query("unit"), Priority: c.Query("priority"), Range: c.Query("range"), Limit: limit})
	if err := logs.Validate(query); err != nil {
		return c.Status(fiber.StatusUnprocessableEntity).JSON(fiber.Map{"error": "logs_query_invalid"})
	}
	ctx, cancel := context.WithTimeout(c.UserContext(), 15*time.Second)
	defer cancel()
	entries, err := a.Logs.List(ctx, query)
	if err != nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"error": "logs_query_failed"})
	}
	return c.JSON(entries)
}
