package metrics

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/kantaevsherhan/mini-ubuntu-server-panel/backend/internal/database"
	"gorm.io/gorm"
)

type Collector struct {
	db        *gorm.DB
	interval  time.Duration
	previous  cpuTimes
	hasSample bool
}

type cpuTimes struct {
	total uint64
	idle  uint64
}

func NewCollector(db *gorm.DB, interval time.Duration) *Collector {
	return &Collector{db: db, interval: interval}
}

func (c *Collector) Start(ctx context.Context) {
	_ = c.collect(ctx)
	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			_ = c.collect(ctx)
		}
	}
}

func (c *Collector) collect(ctx context.Context) error {
	cpu, err := readCPU()
	if err != nil {
		return err
	}
	used, total, memoryPercent, err := readMemory()
	if err != nil {
		return err
	}
	if !c.hasSample {
		c.previous = cpu
		c.hasSample = true
		return nil
	}
	totalDelta := cpu.total - c.previous.total
	idleDelta := cpu.idle - c.previous.idle
	c.previous = cpu
	if totalDelta == 0 || idleDelta > totalDelta {
		return errors.New("invalid CPU counters")
	}
	cpuPercent := float64(totalDelta-idleDelta) / float64(totalDelta) * 100
	sample := database.MetricSample{SampledAt: time.Now().UTC(), CPUPercent: cpuPercent, MemoryPercent: memoryPercent, MemoryUsedBytes: used, MemoryTotalBytes: total}
	return c.db.WithContext(ctx).Create(&sample).Error
}

func readCPU() (cpuTimes, error) {
	file, err := os.Open("/proc/stat")
	if err != nil {
		return cpuTimes{}, err
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	if !scanner.Scan() {
		return cpuTimes{}, errors.New("missing aggregate CPU row")
	}
	fields := strings.Fields(scanner.Text())
	if len(fields) < 8 || fields[0] != "cpu" {
		return cpuTimes{}, errors.New("invalid /proc/stat CPU row")
	}
	values := make([]uint64, len(fields)-1)
	for i, field := range fields[1:] {
		values[i], err = strconv.ParseUint(field, 10, 64)
		if err != nil {
			return cpuTimes{}, fmt.Errorf("parse CPU counter: %w", err)
		}
	}
	var total uint64
	for _, value := range values {
		total += value
	}
	return cpuTimes{total: total, idle: values[3] + values[4]}, nil
}

func readMemory() (used, total uint64, percent float64, err error) {
	file, err := os.Open("/proc/meminfo")
	if err != nil {
		return 0, 0, 0, err
	}
	defer file.Close()
	values := map[string]uint64{}
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 2 {
			continue
		}
		key := strings.TrimSuffix(fields[0], ":")
		if key != "MemTotal" && key != "MemAvailable" {
			continue
		}
		value, parseErr := strconv.ParseUint(fields[1], 10, 64)
		if parseErr != nil {
			return 0, 0, 0, parseErr
		}
		values[key] = value * 1024
	}
	if err := scanner.Err(); err != nil {
		return 0, 0, 0, err
	}
	total = values["MemTotal"]
	available := values["MemAvailable"]
	if total == 0 || available > total {
		return 0, 0, 0, errors.New("invalid /proc/meminfo values")
	}
	used = total - available
	return used, total, float64(used) / float64(total) * 100, nil
}
