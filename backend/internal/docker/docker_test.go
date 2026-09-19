package docker

import (
	"context"
	"testing"

	containertypes "github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
)

type fakeEngine struct {
	engineClient
	action  string
	id      string
	created client.ContainerCreateOptions
}

func (f *fakeEngine) ContainerList(context.Context, client.ContainerListOptions) (client.ContainerListResult, error) {
	return client.ContainerListResult{Items: []containertypes.Summary{{ID: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef", Names: []string{"/web"}, Image: "nginx:latest", State: containertypes.StateRunning, Status: "Up 2 hours", Created: 100}}}, nil
}
func (f *fakeEngine) ContainerStart(_ context.Context, id string, _ client.ContainerStartOptions) (client.ContainerStartResult, error) {
	f.id, f.action = id, "start"
	return client.ContainerStartResult{}, nil
}
func (f *fakeEngine) ContainerStop(_ context.Context, id string, _ client.ContainerStopOptions) (client.ContainerStopResult, error) {
	f.id, f.action = id, "stop"
	return client.ContainerStopResult{}, nil
}
func (f *fakeEngine) ContainerRestart(_ context.Context, id string, _ client.ContainerRestartOptions) (client.ContainerRestartResult, error) {
	f.id, f.action = id, "restart"
	return client.ContainerRestartResult{}, nil
}
func (f *fakeEngine) ContainerRemove(_ context.Context, id string, _ client.ContainerRemoveOptions) (client.ContainerRemoveResult, error) {
	f.id, f.action = id, "remove"
	return client.ContainerRemoveResult{}, nil
}

func TestManagerListsAndControlsContainers(t *testing.T) {
	engine := &fakeEngine{}
	manager := NewManagerWithClient(engine)
	items, err := manager.List(context.Background())
	if err != nil || len(items) != 1 || items[0].Name != "web" || items[0].State != "running" {
		t.Fatalf("unexpected list: items=%#v err=%v", items, err)
	}
	id := items[0].ID
	if err := manager.Action(context.Background(), id, "restart"); err != nil || engine.id != id || engine.action != "restart" {
		t.Fatalf("unexpected action: id=%q action=%q err=%v", engine.id, engine.action, err)
	}
}

func TestValidateActionRejectsUnsafeValues(t *testing.T) {
	for _, value := range []struct{ id, action string }{{"nginx", "start"}, {"../../socket", "stop"}, {"0123456789ab", "exec"}} {
		if err := ValidateAction(value.id, value.action); err == nil {
			t.Fatalf("unsafe action accepted: %#v", value)
		}
	}
}

func (f *fakeEngine) ContainerCreate(_ context.Context, options client.ContainerCreateOptions) (client.ContainerCreateResult, error) {
	f.created = options
	return client.ContainerCreateResult{ID: "abcdefabcdef"}, nil
}

func TestRunBuildsUnprivilegedContainer(t *testing.T) {
	engine := &fakeEngine{}
	manager := NewManagerWithClient(engine)
	id, err := manager.Run(context.Background(), RunRequest{
		Image: "nginx:1.27", Name: "web", RestartPolicy: "unless-stopped", Start: true,
		Ports:   []PortMapping{{HostPort: 8080, ContainerPort: 80, Protocol: "tcp"}},
		Env:     []string{"MODE=prod"},
		Volumes: []VolumeMapping{{Source: "webdata", Target: "/usr/share/nginx/html"}},
	})
	if err != nil || id != "abcdefabcdef" || engine.action != "start" {
		t.Fatalf("unexpected run: id=%q action=%q err=%v", id, engine.action, err)
	}
	host := engine.created.HostConfig
	if host.Privileged || string(host.RestartPolicy.Name) != "unless-stopped" || len(host.PortBindings) != 1 || len(host.Mounts) != 1 {
		t.Fatalf("unexpected host config: %#v", host)
	}
}

func TestValidateRunRequestRejectsUnsafeValues(t *testing.T) {
	cases := []RunRequest{
		{Image: ""},
		{Image: "nginx;rm -rf /"},
		{Image: "nginx", Name: "../x"},
		{Image: "nginx", RestartPolicy: "sometimes"},
		{Image: "nginx", Ports: []PortMapping{{HostPort: 70000, ContainerPort: 80, Protocol: "tcp"}}},
		{Image: "nginx", Ports: []PortMapping{{HostPort: 80, ContainerPort: 80, Protocol: "sctp"}}},
		{Image: "nginx", Env: []string{"BAD NAME=1"}},
		{Image: "nginx", Volumes: []VolumeMapping{{Source: "/", Target: "/host"}}},
		{Image: "nginx", Volumes: []VolumeMapping{{Source: "/etc/ssh", Target: "/x"}}},
		{Image: "nginx", Volumes: []VolumeMapping{{Source: "/var/run/docker.sock", Target: "/x"}}},
		{Image: "nginx", Volumes: []VolumeMapping{{Source: "/srv/app", Target: "relative"}}},
	}
	for _, request := range cases {
		if err := ValidateRunRequest(request); err == nil {
			t.Fatalf("unsafe request accepted: %#v", request)
		}
	}
	if err := ValidateRunRequest(RunRequest{Image: "ghcr.io/org/app@sha256:abc", Volumes: []VolumeMapping{{Source: "/srv/app", Target: "/data"}}}); err != nil {
		t.Fatalf("valid request rejected: %v", err)
	}
}

func TestPruneRejectsUnknownKind(t *testing.T) {
	if _, err := NewManagerWithClient(&fakeEngine{}).Prune(context.Background(), "system"); err == nil {
		t.Fatal("unknown prune kind accepted")
	}
}
