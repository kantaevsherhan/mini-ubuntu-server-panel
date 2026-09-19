// Package monitor watches Docker, systemd and host resources and turns state changes into notification events.
package monitor

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/kantaevsherhan/mini-ubuntu-server-panel/backend/internal/database"
	dockermanager "github.com/kantaevsherhan/mini-ubuntu-server-panel/backend/internal/docker"
	"github.com/kantaevsherhan/mini-ubuntu-server-panel/backend/internal/notifications"
	"github.com/kantaevsherhan/mini-ubuntu-server-panel/backend/internal/services"
	"github.com/kantaevsherhan/mini-ubuntu-server-panel/backend/internal/sysinfo"
	"gorm.io/gorm"
)

// Settings are edited from the Notifications page and stored as JSON in monitor_settings.
type Settings struct {
	IntervalSeconds  int      `json:"interval_seconds"`
	DockerEnabled    bool     `json:"docker_enabled"`
	DockerIgnore     []string `json:"docker_ignore"`
	ServicesEnabled  bool     `json:"services_enabled"`
	ServicesWatch    []string `json:"services_watch"`
	ResourcesEnabled bool     `json:"resources_enabled"`
	CPUPercent       float64  `json:"cpu_percent"`
	MemoryPercent    float64  `json:"memory_percent"`
	SwapPercent      float64  `json:"swap_percent"`
	DiskPercent      float64  `json:"disk_percent"`
}

func DefaultSettings() Settings {
	return Settings{IntervalSeconds: 60, DockerEnabled: true, DockerIgnore: []string{}, ServicesEnabled: true, ServicesWatch: []string{}, ResourcesEnabled: true, CPUPercent: 90, MemoryPercent: 90, SwapPercent: 80, DiskPercent: 90}
}

func (s Settings) Validate() error {
	if s.IntervalSeconds < 15 || s.IntervalSeconds > 3600 {
		return errors.New("interval must be between 15 and 3600 seconds")
	}
	for _, value := range []float64{s.CPUPercent, s.MemoryPercent, s.SwapPercent, s.DiskPercent} {
		if value < 10 || value > 100 {
			return errors.New("thresholds must be between 10 and 100 percent")
		}
	}
	if len(s.DockerIgnore) > 200 || len(s.ServicesWatch) > 200 {
		return errors.New("too many entries")
	}
	for _, name := range append(slices.Clone(s.DockerIgnore), s.ServicesWatch...) {
		if name == "" || len(name) > 256 || strings.ContainsAny(name, "\x00\n") {
			return errors.New("invalid name")
		}
	}
	return nil
}

func LoadSettings(ctx context.Context, db *gorm.DB) Settings {
	settings := DefaultSettings()
	var row database.MonitorSetting
	if db.WithContext(ctx).First(&row, 1).Error == nil {
		_ = json.Unmarshal([]byte(row.SettingsJSON), &settings)
	}
	if settings.DockerIgnore == nil {
		settings.DockerIgnore = []string{}
	}
	if settings.ServicesWatch == nil {
		settings.ServicesWatch = []string{}
	}
	return settings
}

func SaveSettings(ctx context.Context, db *gorm.DB, settings Settings) error {
	if err := settings.Validate(); err != nil {
		return err
	}
	data, err := json.Marshal(settings)
	if err != nil {
		return err
	}
	return db.WithContext(ctx).Save(&database.MonitorSetting{ID: 1, SettingsJSON: string(data), UpdatedAt: time.Now().UTC()}).Error
}

type Notifier interface {
	Enqueue(context.Context, notifications.Event) (int64, error)
}

type Monitor struct {
	DB       *gorm.DB
	Notifier Notifier
	Docker   dockermanager.Controller
	Services services.Controller
	System   *sysinfo.Collector
	Hostname string

	mu         sync.Mutex
	containers map[string]string // container name -> last observed state
	alerted    map[string]bool   // event key + "|" + subject currently alerting
	over       map[string]int    // consecutive checks above a resource threshold
	wake       chan struct{}
}

func New(db *gorm.DB, notifier Notifier, docker dockermanager.Controller, units services.Controller) *Monitor {
	hostname, _ := os.Hostname()
	return &Monitor{DB: db, Notifier: notifier, Docker: docker, Services: units, System: sysinfo.New(), Hostname: hostname, wake: make(chan struct{}, 1)}
}

// Reload makes the next check happen now, e.g. after settings were saved.
func (m *Monitor) Reload() {
	select {
	case m.wake <- struct{}{}:
	default:
	}
}

func (m *Monitor) Run(ctx context.Context) {
	m.seed(ctx)
	m.emit(ctx, notifications.Event{Key: "system.started", Severity: "info", Instant: true, DedupKey: fmt.Sprintf("system.started:%d", time.Now().UnixNano()),
		Payload: m.payload("Панель запущена", "Мониторинг сервера активен.")})
	for {
		settings := LoadSettings(ctx, m.DB)
		m.Check(ctx, settings)
		timer := time.NewTimer(time.Duration(settings.IntervalSeconds) * time.Second)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-m.wake:
			timer.Stop()
		case <-timer.C:
		}
	}
}

// seed restores which subjects are still alerting so recoveries are sent after a panel restart.
func (m *Monitor) seed(ctx context.Context) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.containers, m.alerted, m.over = map[string]string{}, map[string]bool{}, map[string]int{}
	var states []database.NotificationSubjectState
	m.DB.WithContext(ctx).Where("active = ?", true).Find(&states)
	for _, state := range states {
		m.alerted[state.EventKey+"|"+state.Subject] = true
	}
}

func (m *Monitor) Check(ctx context.Context, settings Settings) {
	if m.containers == nil {
		m.seed(ctx)
	}
	if settings.DockerEnabled && m.Docker != nil {
		m.checkDocker(ctx, settings)
	}
	if settings.ServicesEnabled && m.Services != nil {
		m.checkServices(ctx, settings)
	}
	if settings.ResourcesEnabled {
		m.checkResources(ctx, settings)
	}
}

func (m *Monitor) payload(title, message string) map[string]any {
	return map[string]any{"title": title, "message": fmt.Sprintf("%s\n\n🖥 %s · %s", message, m.Hostname, time.Now().Format("02.01.2006 15:04:05"))}
}

func (m *Monitor) emit(ctx context.Context, event notifications.Event) {
	if m.Notifier == nil {
		return
	}
	if _, err := m.Notifier.Enqueue(ctx, event); err != nil {
		log.Printf("monitor: enqueue %s: %v", event.Key, err)
	}
}

// raise alerts once per subject; resolve sends the recovery only if an alert is outstanding.
func (m *Monitor) raise(ctx context.Context, key, severity, subject, title, message string) {
	m.mu.Lock()
	m.alerted[key+"|"+subject] = true
	m.mu.Unlock()
	m.emit(ctx, notifications.Event{Key: key, Severity: severity, Subject: subject, DedupKey: key + ":" + subject, Payload: m.payload(title, message)})
}

func (m *Monitor) resolve(ctx context.Context, key, subject, title, message string) {
	m.mu.Lock()
	active := m.alerted[key+"|"+subject]
	delete(m.alerted, key+"|"+subject)
	m.mu.Unlock()
	if active {
		m.emit(ctx, notifications.Event{Key: key, Severity: "recovery", Subject: subject, Recovery: true, DedupKey: key + ":" + subject, Payload: m.payload(title, message)})
	}
}

func (m *Monitor) checkDocker(ctx context.Context, settings Settings) {
	listCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	containers, err := m.Docker.List(listCtx)
	if err != nil {
		return
	}
	current := map[string]string{}
	for _, container := range containers {
		if slices.Contains(settings.DockerIgnore, container.Name) {
			continue
		}
		current[container.Name] = container.State
		m.mu.Lock()
		previous, known := m.containers[container.Name]
		m.mu.Unlock()
		running := container.State == "running"
		switch {
		case running:
			m.resolve(ctx, "docker.container.stopped", container.Name, "Контейнер снова работает: "+container.Name, container.Status)
		case known && (previous == "running" || previous == "restarting") && (container.State == "exited" || container.State == "dead"):
			m.raise(ctx, "docker.container.stopped", "error", container.Name, "Контейнер остановился: "+container.Name,
				fmt.Sprintf("Образ: %s\nСтатус: %s", container.Image, container.Status))
		}
		if container.State == "restarting" && previous != "restarting" {
			m.emit(ctx, notifications.Event{Key: "docker.container.restarted", Severity: "warning", Instant: true,
				DedupKey: fmt.Sprintf("docker.container.restarted:%s:%d", container.Name, time.Now().UnixNano()),
				Payload:  m.payload("Контейнер перезапускается: "+container.Name, container.Status)})
		}
		// Docker keeps the last health status of stopped containers; only running ones can be unhealthy.
		if running && container.Health == "unhealthy" {
			m.raise(ctx, "docker.container.unhealthy", "critical", container.Name, "Контейнер unhealthy: "+container.Name, container.Status)
		} else if running {
			m.resolve(ctx, "docker.container.unhealthy", container.Name, "Контейнер снова healthy: "+container.Name, container.Status)
		}
	}
	m.mu.Lock()
	m.containers = current
	m.mu.Unlock()
}

func (m *Monitor) checkServices(ctx context.Context, settings Settings) {
	listCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	units, err := m.Services.List(listCtx)
	if err != nil {
		return
	}
	for _, unit := range units {
		watched := slices.Contains(settings.ServicesWatch, unit.Name)
		if unit.ActiveState == "failed" {
			m.raise(ctx, "systemd.service.failed", "critical", unit.Name, "Сервис упал: "+unit.Name, unit.Description)
		} else {
			m.resolve(ctx, "systemd.service.failed", unit.Name, "Сервис восстановлен: "+unit.Name, unit.ActiveState)
		}
		if !watched {
			continue
		}
		if unit.ActiveState == "inactive" {
			m.raise(ctx, "systemd.service.stopped", "error", unit.Name, "Сервис остановлен: "+unit.Name, unit.Description)
		} else if unit.ActiveState == "active" {
			m.resolve(ctx, "systemd.service.stopped", unit.Name, "Сервис снова работает: "+unit.Name, unit.Description)
		}
	}
}

// threshold alerts after two consecutive checks above the limit and recovers 5 points below it.
func (m *Monitor) threshold(ctx context.Context, key, severity, subject, label string, value, limit float64) {
	m.mu.Lock()
	switch {
	case value >= limit:
		m.over[key+"|"+subject]++
	case value < limit-5:
		m.over[key+"|"+subject] = 0
	}
	count := m.over[key+"|"+subject]
	m.mu.Unlock()
	if count >= 2 {
		m.raise(ctx, key, severity, subject, fmt.Sprintf("%s: %.0f%%", label, value), fmt.Sprintf("Порог: %.0f%%", limit))
	} else if count == 0 {
		m.resolve(ctx, key, subject, fmt.Sprintf("%s в норме: %.0f%%", label, value), fmt.Sprintf("Порог: %.0f%%", limit))
	}
}

func (m *Monitor) checkResources(ctx context.Context, settings Settings) {
	info := m.System.Info()
	var sample database.MetricSample
	if m.DB.WithContext(ctx).Order("sampled_at DESC").First(&sample).Error == nil && time.Since(sample.SampledAt) < 5*time.Minute {
		m.threshold(ctx, "resource.cpu.high", "critical", "", "Высокая загрузка CPU", sample.CPUPercent, settings.CPUPercent)
	}
	if info.Memory.TotalBytes > 0 {
		used := float64(info.Memory.TotalBytes-info.Memory.AvailableBytes) / float64(info.Memory.TotalBytes) * 100
		m.threshold(ctx, "resource.memory.high", "critical", "", "Мало оперативной памяти", used, settings.MemoryPercent)
	}
	if info.Memory.SwapTotalBytes > 0 {
		used := float64(info.Memory.SwapTotalBytes-info.Memory.SwapFreeBytes) / float64(info.Memory.SwapTotalBytes) * 100
		m.threshold(ctx, "resource.swap.high", "warning", "", "Swap почти заполнен", used, settings.SwapPercent)
	}
	for _, disk := range info.Disks {
		if disk.TotalBytes == 0 {
			continue
		}
		used := float64(disk.UsedBytes) / float64(disk.TotalBytes) * 100
		m.threshold(ctx, "resource.disk.full", "critical", disk.Mountpoint, "Диск заполнен "+disk.Mountpoint, used, settings.DiskPercent)
	}
}
