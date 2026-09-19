package httpapi

import (
	"sync"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/kantaevsherhan/mini-ubuntu-server-panel/backend/internal/sysinfo"
)

// systemCache keeps /proc scans cheap when several open tabs poll at the same time.
var systemCache struct {
	mu      sync.Mutex
	info    sysinfo.Info
	infoAt  time.Time
	ports   []sysinfo.ListeningPort
	portsAt time.Time
}

const systemCacheTTL = 3 * time.Second

func (a API) systemCollector() *sysinfo.Collector {
	if a.System != nil {
		return a.System
	}
	return sysinfo.New()
}

func (a API) systemInfo(c *fiber.Ctx) error {
	systemCache.mu.Lock()
	defer systemCache.mu.Unlock()
	if time.Since(systemCache.infoAt) > systemCacheTTL {
		systemCache.info, systemCache.infoAt = a.systemCollector().Info(), time.Now()
	}
	return c.JSON(systemCache.info)
}

func (a API) systemPorts(c *fiber.Ctx) error {
	systemCache.mu.Lock()
	defer systemCache.mu.Unlock()
	if time.Since(systemCache.portsAt) > systemCacheTTL {
		systemCache.ports, systemCache.portsAt = a.systemCollector().ListeningPorts(), time.Now()
	}
	return c.JSON(systemCache.ports)
}
