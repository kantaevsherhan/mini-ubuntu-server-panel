package docker

import (
	"context"
	"encoding/json"
	"io"
	"math"
	"os"
	"strings"
	"testing"

	ct "github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
)

func TestStatsCalculationAndCounterReset(t *testing.T) {
	var sample ct.StatsResponse
	err := json.Unmarshal([]byte(`{"cpu_stats":{"cpu_usage":{"total_usage":300},"system_cpu_usage":1000,"online_cpus":4},"precpu_stats":{"cpu_usage":{"total_usage":100},"system_cpu_usage":600},"memory_stats":{"usage":1000,"limit":2000,"stats":{"inactive_file":200}},"pids_stats":{"current":6},"networks":{"eth0":{"rx_bytes":100,"tx_bytes":200}},"blkio_stats":{"io_service_bytes_recursive":[{"op":"Read","value":80},{"op":"Write","value":90}]}}`), &sample)
	if err != nil {
		t.Fatal(err)
	}
	s := calculateStats(sample)
	if s.CPUPercent != 200 || s.MemoryUsage != 800 || s.MemoryPercent != 40 || s.NetworkRX != 100 || s.NetworkTX != 200 || s.BlockRead != 80 || s.BlockWrite != 90 || s.PIDs != 6 {
		t.Fatalf("incorrect statistics: %+v", s)
	}
	sample.CPUStats.CPUUsage.TotalUsage = 1
	if s = calculateStats(sample); s.CPUPercent != 0 {
		t.Fatal("counter reset underflow")
	}
}

func TestDockerResourcesIntegration(t *testing.T) {
	id := os.Getenv("MUSP_TEST_CONTAINER_ID")
	if id == "" {
		t.Skip("isolated Docker fixture not configured")
	}
	manager, err := NewManager()
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	inspected, err := manager.client.ContainerInspect(ctx, id, client.ContainerInspectOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if inspected.Container.Name != "/musp-resource-fixture" {
		t.Fatal("refusing to change a non-fixture container")
	}
	if err = manager.UpdateLimits(ctx, id, ResourceLimits{CPUs: 0.5, MemoryMB: 96}); err != nil {
		t.Fatal(err)
	}
	stats, err := manager.Stats(ctx)
	if err != nil {
		t.Fatal(err)
	}
	sample, ok := stats[id]
	if !ok || sample.Error || sample.Limits.CPUs != 0.5 || sample.Limits.MemoryMB != 96 || sample.MemoryLimit != 96*1024*1024 {
		t.Fatalf("incorrect live limits: %+v", sample)
	}
	if err = manager.UpdateLimits(ctx, id, ResourceLimits{}); err != nil {
		t.Fatal(err)
	}
	inspected, err = manager.client.ContainerInspect(ctx, id, client.ContainerInspectOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if inspected.Container.HostConfig.Memory != 96*1024*1024 || inspected.Container.HostConfig.CPUQuota > 0 {
		t.Fatal("CPU reset / unchanged RAM failed")
	}
}

func TestValidateResourceLimits(t *testing.T) {
	for _, l := range []ResourceLimits{{CPUs: -1}, {CPUs: math.NaN()}, {CPUs: math.Inf(1)}, {CPUs: 0.001}, {MemoryMB: -1}, {MemoryMB: 5}, {MemoryMB: 16777217}} {
		if ValidateLimits(l) == nil {
			t.Fatalf("invalid limit accepted: %+v", l)
		}
	}
	for _, l := range []ResourceLimits{{}, {CPUs: 0.5, MemoryMB: 128}} {
		if err := ValidateLimits(l); err != nil {
			t.Fatal(err)
		}
	}
}

type resourceFake struct {
	fakeEngine
	update client.ContainerUpdateOptions
}

func (f *resourceFake) ContainerInspect(context.Context, string, client.ContainerInspectOptions) (client.ContainerInspectResult, error) {
	var result client.ContainerInspectResult
	// Use JSON to mirror the daemon payload without coupling to inspect embedding.
	err := json.Unmarshal([]byte(`{"HostConfig":{"Memory":134217728,"CpuQuota":50000,"CpuPeriod":100000}}`), &result.Container)
	return result, err
}
func (f *resourceFake) ContainerStats(context.Context, string, client.ContainerStatsOptions) (client.ContainerStatsResult, error) {
	return client.ContainerStatsResult{Body: io.NopCloser(strings.NewReader(`{"memory_stats":{"usage":1024,"limit":134217728}}`))}, nil
}
func (f *resourceFake) ContainerUpdate(_ context.Context, _ string, o client.ContainerUpdateOptions) (client.ContainerUpdateResult, error) {
	f.update = o
	return client.ContainerUpdateResult{}, nil
}

func TestStatsAndLimitsRoundTrip(t *testing.T) {
	engine := &resourceFake{}
	manager := NewManagerWithClient(engine)
	stats, err := manager.Stats(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range stats {
		if s.Error || s.Limits.MemoryMB != 128 || s.Limits.CPUs != 0.5 || s.MemoryUsage != 1024 {
			t.Fatalf("unexpected: %+v", s)
		}
	}
	if err = manager.UpdateLimits(context.Background(), "abcdefabcdef", ResourceLimits{CPUs: 1.5, MemoryMB: 256}); err != nil {
		t.Fatal(err)
	}
	if engine.update.Resources.CPUQuota != 150000 || engine.update.Resources.Memory != 268435456 {
		t.Fatal("limits not sent")
	}
	if err = manager.UpdateLimits(context.Background(), "abcdefabcdef", ResourceLimits{}); err != nil {
		t.Fatal(err)
	}
	if engine.update.Resources.CPUQuota != -1 || engine.update.Resources.Memory != 0 {
		t.Fatal("CPU reset / unchanged RAM not sent")
	}
	if err = manager.UpdateLimits(context.Background(), "../../docker.sock", ResourceLimits{}); err == nil {
		t.Fatal("unsafe id accepted")
	}
}
