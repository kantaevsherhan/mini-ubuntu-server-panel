package httpapi

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/kantaevsherhan/mini-ubuntu-server-panel/backend/internal/auth"
	"github.com/kantaevsherhan/mini-ubuntu-server-panel/backend/internal/database"
	"github.com/kantaevsherhan/mini-ubuntu-server-panel/backend/internal/firewall"
)

func (a API) firewallStatus(c *fiber.Ctx) error {
	if a.Firewall == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"error": "firewall_service_unavailable"})
	}
	ctx, cancel := context.WithTimeout(c.UserContext(), 10*time.Second)
	defer cancel()
	status, err := a.Firewall.Status(ctx)
	if err != nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"error": "firewall_unavailable"})
	}
	return c.JSON(status)
}

func (a API) firewallAddRule(c *fiber.Ctx) error {
	if a.Firewall == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"error": "firewall_service_unavailable"})
	}
	var request firewall.AddRequest
	if err := c.BodyParser(&request); err != nil {
		return fiber.ErrBadRequest
	}
	request.Action = strings.ToLower(strings.TrimSpace(request.Action))
	request.Protocol = strings.ToLower(strings.TrimSpace(request.Protocol))
	request.Source = strings.TrimSpace(request.Source)
	if err := firewall.ValidateRule(request); err != nil {
		return c.Status(fiber.StatusUnprocessableEntity).JSON(fiber.Map{"error": "firewall_rule_invalid"})
	}
	ctx, cancel := context.WithTimeout(c.UserContext(), 15*time.Second)
	defer cancel()
	if err := a.Firewall.Add(ctx, request); err != nil {
		return c.Status(fiber.StatusConflict).JSON(fiber.Map{"error": "firewall_rule_add_failed"})
	}
	claims := c.Locals("claims").(*auth.Claims)
	details := fmt.Sprintf(`{"action":%q,"port":%d,"protocol":%q,"source":%q}`, request.Action, request.Port, request.Protocol, request.Source)
	database.Audit(a.DB, claims.UserID, "firewall.rule.add", "firewall_rule", fmt.Sprintf("%s/%d", request.Protocol, request.Port), details, c.IP())
	return c.SendStatus(fiber.StatusNoContent)
}

func (a API) firewallDeleteRule(c *fiber.Ctx) error {
	if a.Firewall == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"error": "firewall_service_unavailable"})
	}
	number, err := strconv.Atoi(c.Params("number"))
	if err != nil || number < 1 || number > 10000 {
		return c.Status(fiber.StatusUnprocessableEntity).JSON(fiber.Map{"error": "firewall_rule_number_invalid"})
	}
	ctx, cancel := context.WithTimeout(c.UserContext(), 15*time.Second)
	defer cancel()
	if err := a.Firewall.Delete(ctx, number); err != nil {
		return c.Status(fiber.StatusConflict).JSON(fiber.Map{"error": "firewall_rule_delete_failed"})
	}
	claims := c.Locals("claims").(*auth.Claims)
	database.Audit(a.DB, claims.UserID, "firewall.rule.delete", "firewall_rule", strconv.Itoa(number), `{"deleted":true}`, c.IP())
	return c.SendStatus(fiber.StatusNoContent)
}
