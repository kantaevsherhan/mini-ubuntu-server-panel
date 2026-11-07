package docker

import (
	"context"
	"encoding/json"
	"io"
	"math"
	"strings"
	"sync"
	"time"

	ct "github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
)

type ResourceLimits struct {
	CPUs     float64 `json:"cpus"`
	MemoryMB int64   `json:"memory_mb"`
}

func ValidateLimits(l ResourceLimits) error {
	if math.IsNaN(l.CPUs) || math.IsInf(l.CPUs, 0) || l.CPUs < 0 || l.CPUs > 1024 || (l.CPUs > 0 && l.CPUs < 0.01) || l.MemoryMB < 0 || (l.MemoryMB > 0 && l.MemoryMB < 6) || l.MemoryMB > 16777216 {
		return ErrInvalid
	}
	return nil
}

func (l ResourceLimits) resources(update bool) ct.Resources {
	r := ct.Resources{CPUPeriod: 100000, CPUQuota: int64(l.CPUs * 100000), Memory: l.MemoryMB * 1024 * 1024}
	if update {
		if l.CPUs == 0 {
			r.CPUQuota = -1
		}
		r.MemorySwap = -1
	}
	return r
}

type ContainerStats struct {
	CPUPercent    float64        `json:"cpu_percent"`
	MemoryUsage   uint64         `json:"memory_usage"`
	MemoryLimit   uint64         `json:"memory_limit"`
	MemoryPercent float64        `json:"memory_percent"`
	NetworkRX     uint64         `json:"network_rx"`
	NetworkTX     uint64         `json:"network_tx"`
	BlockRead     uint64         `json:"block_read"`
	BlockWrite    uint64         `json:"block_write"`
	PIDs          uint64         `json:"pids"`
	Limits        ResourceLimits `json:"limits"`
	Error         bool           `json:"error"`
	SampledAt     time.Time      `json:"sampled_at"`
}

type resourceEngine interface {
	ContainerStats(context.Context, string, client.ContainerStatsOptions) (client.ContainerStatsResult, error)
	ContainerUpdate(context.Context, string, client.ContainerUpdateOptions) (client.ContainerUpdateResult, error)
}

const statsWorkers = 12

// Stats uses bounded workers and a deadline shared by the entire snapshot.
func (m Manager) Stats(ctx context.Context) (map[string]ContainerStats, error) {
	engine, ok := m.client.(resourceEngine)
	if !ok {
		return nil, ErrInvalid
	}
	items, err := m.List(ctx)
	if err != nil {
		return nil, err
	}
	result := make(map[string]ContainerStats, len(items))
	var mu sync.Mutex
	var wg sync.WaitGroup
	jobs := make(chan Container)
	// Each running container costs about a second (Docker samples CPU twice), mostly waiting:
	// enough workers keep a typical host under two seconds while still bounding the fan-out.
	workers := max(1, min(len(items), statsWorkers))
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for item := range jobs {
				value := ContainerStats{}
				inspect, e := m.client.ContainerInspect(ctx, item.ID, client.ContainerInspectOptions{})
				if e != nil {
					value.Error = true
				} else if inspect.Container.HostConfig != nil {
					r := inspect.Container.HostConfig.Resources
					if r.Memory > 0 {
						value.Limits.MemoryMB = r.Memory / 1024 / 1024
					}
					if r.NanoCPUs > 0 {
						value.Limits.CPUs = float64(r.NanoCPUs) / 1e9
					} else if r.CPUQuota > 0 && r.CPUPeriod > 0 {
						value.Limits.CPUs = float64(r.CPUQuota) / float64(r.CPUPeriod)
					}
				}
				if item.State == "running" {
					sample, e := engine.ContainerStats(ctx, item.ID, client.ContainerStatsOptions{IncludePreviousSample: true})
					if e != nil {
						value.Error = true
					} else {
						var raw ct.StatsResponse
						e = json.NewDecoder(io.LimitReader(sample.Body, 2<<20)).Decode(&raw)
						_ = sample.Body.Close()
						if e != nil {
							value.Error = true
						} else {
							limits := value.Limits
							failed := value.Error
							value = calculateStats(raw)
							value.Limits = limits
							value.Error = failed
						}
					}
				}
				mu.Lock()
				result[item.ID] = value
				mu.Unlock()
			}
		}()
	}
	for _, item := range items {
		select {
		case jobs <- item:
		case <-ctx.Done():
			close(jobs)
			wg.Wait()
			return nil, ctx.Err()
		}
	}
	close(jobs)
	wg.Wait()
	return result, nil
}

func calculateStats(s ct.StatsResponse) ContainerStats {
	v := ContainerStats{MemoryUsage: s.MemoryStats.Usage, MemoryLimit: s.MemoryStats.Limit, PIDs: s.PidsStats.Current, SampledAt: s.Read}
	cache := s.MemoryStats.Stats["total_inactive_file"]
	if n, ok := s.MemoryStats.Stats["inactive_file"]; ok {
		cache = n
	}
	if cache < v.MemoryUsage {
		v.MemoryUsage -= cache
	}
	if v.MemoryLimit > 0 {
		v.MemoryPercent = float64(v.MemoryUsage) / float64(v.MemoryLimit) * 100
	}
	if s.CPUStats.CPUUsage.TotalUsage >= s.PreCPUStats.CPUUsage.TotalUsage && s.CPUStats.SystemUsage > s.PreCPUStats.SystemUsage {
		cpus := s.CPUStats.OnlineCPUs
		if cpus == 0 {
			cpus = uint32(len(s.CPUStats.CPUUsage.PercpuUsage))
		}
		v.CPUPercent = float64(s.CPUStats.CPUUsage.TotalUsage-s.PreCPUStats.CPUUsage.TotalUsage) / float64(s.CPUStats.SystemUsage-s.PreCPUStats.SystemUsage) * float64(cpus) * 100
	}
	for _, n := range s.Networks {
		v.NetworkRX += n.RxBytes
		v.NetworkTX += n.TxBytes
	}
	for _, b := range s.BlkioStats.IoServiceBytesRecursive {
		switch strings.ToLower(b.Op) {
		case "read":
			v.BlockRead += b.Value
		case "write":
			v.BlockWrite += b.Value
		}
	}
	return v
}

func (m Manager) UpdateLimits(ctx context.Context, id string, l ResourceLimits) error {
	if !containerIDPattern.MatchString(id) {
		return ErrInvalid
	}
	if err := ValidateLimits(l); err != nil {
		return err
	}
	engine, ok := m.client.(resourceEngine)
	if !ok {
		return ErrInvalid
	}
	// Docker cannot update NanoCPUs and quota together. Keep the existing mechanism.
	inspected, err := m.client.ContainerInspect(ctx, id, client.ContainerInspectOptions{})
	if err != nil {
		return mapError(err)
	}
	r := l.resources(true)
	if inspected.Container.HostConfig != nil && inspected.Container.HostConfig.NanoCPUs > 0 {
		// A zero NanoCPUs is omitted by Docker's update API; use quota only after clearing it.
		if l.CPUs == 0 {
			return ErrConflict
		}
		r.CPUPeriod = 0
		r.CPUQuota = 0
		r.NanoCPUs = int64(l.CPUs * 1e9)
	}
	_, err = engine.ContainerUpdate(ctx, id, client.ContainerUpdateOptions{Resources: &r})
	return mapError(err)
}
