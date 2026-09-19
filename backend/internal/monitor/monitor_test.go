package monitor

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/kantaevsherhan/mini-ubuntu-server-panel/backend/internal/database"
	dockermanager "github.com/kantaevsherhan/mini-ubuntu-server-panel/backend/internal/docker"
	"github.com/kantaevsherhan/mini-ubuntu-server-panel/backend/internal/notifications"
	"github.com/kantaevsherhan/mini-ubuntu-server-panel/backend/internal/services"
	"github.com/kantaevsherhan/mini-ubuntu-server-panel/backend/internal/sysinfo"
)

type recorder struct{ events []notifications.Event }

func (r *recorder) Enqueue(_ context.Context, event notifications.Event) (int64, error) {
	r.events = append(r.events, event)
	return 1, nil
}

type fakeDocker struct {
	dockermanager.Controller
	items []dockermanager.Container
}

func (f *fakeDocker) List(context.Context) ([]dockermanager.Container, error) { return f.items, nil }

type fakeUnits struct {
	services.Controller
	items []services.Service
}

func (f *fakeUnits) List(context.Context) ([]services.Service, error) { return f.items, nil }

func keys(events []notifications.Event) []string {
	result := []string{}
	for _, event := range events {
		name := event.Key + ":" + event.Subject
		if event.Recovery {
			name += ":recovery"
		}
		result = append(result, name)
	}
	return result
}

func TestMonitorAlertsOnContainerTransitionsOnly(t *testing.T) {
	db, err := database.Open(filepath.Join(t.TempDir(), "panel.db"))
	if err != nil {
		t.Fatal(err)
	}
	events := &recorder{}
	docker := &fakeDocker{items: []dockermanager.Container{{Name: "web", State: "running"}, {Name: "old", State: "exited", Health: "unhealthy"}, {Name: "noisy", State: "running"}}}
	units := &fakeUnits{items: []services.Service{{Name: "nginx.service", ActiveState: "active"}}}
	monitor := New(db, events, docker, units)
	monitor.System = &sysinfo.Collector{Root: t.TempDir()}
	settings := DefaultSettings()
	settings.DockerIgnore = []string{"noisy"}
	ctx := context.Background()

	monitor.Check(ctx, settings)
	if len(events.events) != 0 {
		t.Fatalf("baseline must not alert (already exited containers are ignored): %v", keys(events.events))
	}

	docker.items = []dockermanager.Container{{Name: "web", State: "exited"}, {Name: "old", State: "exited"}, {Name: "noisy", State: "exited"}}
	units.items[0].ActiveState = "failed"
	monitor.Check(ctx, settings)
	got := keys(events.events)
	if len(got) != 2 || got[0] != "docker.container.stopped:web" || got[1] != "systemd.service.failed:nginx.service" {
		t.Fatalf("unexpected alerts: %v", got)
	}

	docker.items[0].State = "running"
	units.items[0].ActiveState = "active"
	monitor.Check(ctx, settings)
	got = keys(events.events)
	if len(got) != 4 || got[2] != "docker.container.stopped:web:recovery" || got[3] != "systemd.service.failed:nginx.service:recovery" {
		t.Fatalf("unexpected recoveries: %v", got)
	}

	monitor.Check(ctx, settings)
	if len(events.events) != 4 {
		t.Fatalf("steady state must be quiet: %v", keys(events.events))
	}
}

func TestSettingsRoundTripAndValidation(t *testing.T) {
	db, err := database.Open(filepath.Join(t.TempDir(), "panel.db"))
	if err != nil {
		t.Fatal(err)
	}
	settings := DefaultSettings()
	settings.IntervalSeconds = 5
	if SaveSettings(context.Background(), db, settings) == nil {
		t.Fatal("too short interval accepted")
	}
	settings.IntervalSeconds = 30
	settings.DockerIgnore = []string{"test-db"}
	if err := SaveSettings(context.Background(), db, settings); err != nil {
		t.Fatal(err)
	}
	loaded := LoadSettings(context.Background(), db)
	if loaded.IntervalSeconds != 30 || len(loaded.DockerIgnore) != 1 {
		t.Fatalf("unexpected settings: %#v", loaded)
	}
}
