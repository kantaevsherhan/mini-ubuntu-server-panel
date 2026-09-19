package httpapi

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/kantaevsherhan/mini-ubuntu-server-panel/backend/internal/auth"
	"github.com/kantaevsherhan/mini-ubuntu-server-panel/backend/internal/database"
	"github.com/kantaevsherhan/mini-ubuntu-server-panel/backend/internal/monitor"
	"github.com/kantaevsherhan/mini-ubuntu-server-panel/backend/internal/notifications"
)

// notify queues a one-shot security notification without blocking the request.
func (a API) notify(key, severity, title, message string) {
	if a.Notifier == nil {
		return
	}
	hostname, _ := os.Hostname()
	event := notifications.Event{Key: key, Severity: severity, Instant: true, DedupKey: fmt.Sprintf("%s:%d", key, time.Now().UnixNano()),
		Payload: map[string]any{"title": title, "message": fmt.Sprintf("%s\n\n🖥 %s · %s", message, hostname, time.Now().Format("02.01.2006 15:04:05"))}}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if _, err := a.Notifier.Enqueue(ctx, event); err != nil {
			log.Printf("notify %s: %v", key, err)
		}
	}()
}

func (a API) monitorSettings(c *fiber.Ctx) error {
	return c.JSON(monitor.LoadSettings(c.UserContext(), a.DB))
}

func (a API) updateMonitorSettings(c *fiber.Ctx) error {
	var settings monitor.Settings
	if err := c.BodyParser(&settings); err != nil {
		return fiber.ErrBadRequest
	}
	if err := monitor.SaveSettings(c.UserContext(), a.DB, settings); err != nil {
		return c.Status(fiber.StatusUnprocessableEntity).JSON(fiber.Map{"error": "monitor_settings_invalid", "message": err.Error()})
	}
	if a.Monitor != nil {
		a.Monitor.Reload()
	}
	claims := c.Locals("claims").(*auth.Claims)
	database.Audit(a.DB, claims.UserID, "notifications.monitor.update", "monitor_settings", "1", "{}", c.IP())
	return c.JSON(settings)
}
