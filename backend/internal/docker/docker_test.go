package docker

import (
	"context"
	"testing"

	containertypes "github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
)

type fakeEngine struct {
	action string
	id     string
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
