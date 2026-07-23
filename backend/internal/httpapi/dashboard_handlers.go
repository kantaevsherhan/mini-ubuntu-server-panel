package httpapi

import (
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/kantaevsherhan/mini-ubuntu-server-panel/backend/internal/database"
	"gorm.io/gorm"
)

func (a API) dashboard(c *fiber.Ctx) error {
	var users, events int
	var userCount, eventCount int64
	if err := a.DB.WithContext(c.UserContext()).Model(&database.User{}).Where("is_active = ?", true).Count(&userCount).Error; err != nil {
		return err
	}
	if err := a.DB.WithContext(c.UserContext()).Model(&database.NotificationEvent{}).Where("status = ?", "pending").Count(&eventCount).Error; err != nil {
		return err
	}
	users, events = int(userCount), int(eventCount)
	hostname, _ := os.Hostname()
	return c.JSON(fiber.Map{"hostname": hostname, "panel_users": users, "pending_notifications": events, "status": "online"})
}
func (a API) metricsHistory(c *fiber.Ctx) error {
	rangeName := c.Query("range", "day")
	now := time.Now().UTC()
	var start time.Time
	var bucketSeconds int64
	switch rangeName {
	case "day":
		start = now.Add(-24 * time.Hour)
		bucketSeconds = 60
	case "week":
		start = now.Add(-7 * 24 * time.Hour)
		bucketSeconds = 15 * 60
	case "month":
		start = now.Add(-30 * 24 * time.Hour)
		bucketSeconds = 60 * 60
	case "all":
		var sample database.MetricSample
		err := a.DB.WithContext(c.UserContext()).Order("sampled_at").First(&sample).Error
		if err == nil {
			start = sample.SampledAt
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		} else {
			start = now
		}
		span := now.Sub(start)
		bucketSeconds = maxInt64(60, int64(span.Seconds()/500))
	default:
		return c.Status(fiber.StatusUnprocessableEntity).JSON(fiber.Map{"error": "metrics_range_invalid"})
	}
	type metricAggregate struct {
		SampledAt        string
		CPUPercent       float64
		MemoryPercent    float64
		MemoryUsedBytes  float64
		MemoryTotalBytes float64
	}
	var aggregates []metricAggregate
	bucketExpr := fmt.Sprintf("CAST(strftime('%%s', sampled_at) AS INTEGER) / %d", bucketSeconds)
	err := a.DB.WithContext(c.UserContext()).Model(&database.MetricSample{}).Select("datetime((CAST(strftime('%s', sampled_at) AS INTEGER) / ?) * ?, 'unixepoch') AS sampled_at, AVG(cpu_percent) AS cpu_percent, AVG(memory_percent) AS memory_percent, AVG(memory_used_bytes) AS memory_used_bytes, MAX(memory_total_bytes) AS memory_total_bytes", bucketSeconds, bucketSeconds).Where("sampled_at >= ?", start).Group(bucketExpr).Order("sampled_at ASC").Limit(1000).Scan(&aggregates).Error
	if err != nil {
		return err
	}
	points := make([]fiber.Map, 0)
	for _, aggregate := range aggregates {
		points = append(points, fiber.Map{"sampled_at": aggregate.SampledAt + "Z", "cpu_percent": aggregate.CPUPercent, "memory_percent": aggregate.MemoryPercent, "memory_used_bytes": uint64(aggregate.MemoryUsedBytes), "memory_total_bytes": uint64(aggregate.MemoryTotalBytes)})
	}
	return c.JSON(fiber.Map{"range": rangeName, "points": points})
}
func maxInt64(left, right int64) int64 {
	if left > right {
		return left
	}
	return right
}
