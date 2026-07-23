package httpapi

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/kantaevsherhan/mini-ubuntu-server-panel/backend/internal/database"
)

func (a API) settingsOverview(c *fiber.Ctx) error {
	hostname, _ := os.Hostname()
	var metricSamples, auditEvents, activeSessions int64
	now := time.Now().UTC()
	if err := a.DB.WithContext(c.UserContext()).Model(&database.MetricSample{}).Count(&metricSamples).Error; err != nil {
		return err
	}
	if err := a.DB.WithContext(c.UserContext()).Model(&database.AuditEvent{}).Count(&auditEvents).Error; err != nil {
		return err
	}
	if err := a.DB.WithContext(c.UserContext()).Model(&database.WebSession{}).Where("revoked_at IS NULL AND expires_at > ?", now).Count(&activeSessions).Error; err != nil {
		return err
	}
	databaseSize := int64(0)
	if info, err := os.Stat(filepath.Join(a.DataDir, "mini-ubuntu-server.db")); err == nil {
		databaseSize = info.Size()
	}
	roots := []any{}
	if a.Files != nil {
		for _, root := range a.Files.Roots() {
			roots = append(roots, root)
		}
	}
	return c.JSON(fiber.Map{
		"hostname": hostname, "version": a.Version, "go_version": runtime.Version(), "os": runtime.GOOS,
		"architecture": runtime.GOARCH, "data_dir": a.DataDir, "log_dir": a.LogDir,
		"database_size_bytes": databaseSize, "metric_samples": metricSamples, "audit_events": auditEvents,
		"active_sessions": activeSessions, "allowed_directories": roots,
	})
}

func (a API) updateStatus(c *fiber.Ctx) error {
	if a.Updates == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"error": "update_check_unavailable"})
	}
	ctx, cancel := context.WithTimeout(c.UserContext(), 12*time.Second)
	defer cancel()
	status, err := a.Updates.Check(ctx, a.Version)
	if err != nil {
		return c.Status(fiber.StatusBadGateway).JSON(fiber.Map{"error": "update_check_failed"})
	}
	return c.JSON(status)
}
