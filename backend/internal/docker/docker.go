package docker

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	containertypes "github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
)

var containerIDPattern = regexp.MustCompile(`^[a-f0-9]{12,64}$`)

type Container struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Image     string    `json:"image"`
	State     string    `json:"state"`
	Status    string    `json:"status"`
	Health    string    `json:"health"`
	Ports     []string  `json:"ports"`
	CreatedAt time.Time `json:"created_at"`
}

type Controller interface {
	List(context.Context) ([]Container, error)
	Action(context.Context, string, string) error
}

type engineClient interface {
	ContainerList(context.Context, client.ContainerListOptions) (client.ContainerListResult, error)
	ContainerStart(context.Context, string, client.ContainerStartOptions) (client.ContainerStartResult, error)
	ContainerStop(context.Context, string, client.ContainerStopOptions) (client.ContainerStopResult, error)
	ContainerRestart(context.Context, string, client.ContainerRestartOptions) (client.ContainerRestartResult, error)
	ContainerRemove(context.Context, string, client.ContainerRemoveOptions) (client.ContainerRemoveResult, error)
}

type Manager struct {
	client engineClient
}

func NewManager() (*Manager, error) {
	apiClient, err := client.New(client.WithHost("unix:///var/run/docker.sock"))
	if err != nil {
		return nil, err
	}
	return &Manager{client: apiClient}, nil
}

func NewManagerWithClient(apiClient engineClient) *Manager {
	return &Manager{client: apiClient}
}

func (m Manager) List(ctx context.Context) ([]Container, error) {
	result, err := m.client.ContainerList(ctx, client.ContainerListOptions{All: true})
	if err != nil {
		return nil, err
	}
	items := make([]Container, 0, len(result.Items))
	for _, summary := range result.Items {
		name := summary.ID[:min(12, len(summary.ID))]
		if len(summary.Names) > 0 {
			name = strings.TrimPrefix(summary.Names[0], "/")
		}
		health := ""
		if summary.Health != nil {
			health = string(summary.Health.Status)
		}
		items = append(items, Container{
			ID: summary.ID, Name: name, Image: summary.Image, State: string(summary.State), Status: summary.Status,
			Health: health, Ports: formatPorts(summary.Ports), CreatedAt: time.Unix(summary.Created, 0).UTC(),
		})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Name < items[j].Name })
	return items, nil
}

func (m Manager) Action(ctx context.Context, id, action string) error {
	if err := ValidateAction(id, action); err != nil {
		return err
	}
	switch action {
	case "start":
		_, err := m.client.ContainerStart(ctx, id, client.ContainerStartOptions{})
		return err
	case "stop":
		timeout := 15
		_, err := m.client.ContainerStop(ctx, id, client.ContainerStopOptions{Timeout: &timeout})
		return err
	case "restart":
		timeout := 15
		_, err := m.client.ContainerRestart(ctx, id, client.ContainerRestartOptions{Timeout: &timeout})
		return err
	case "remove":
		_, err := m.client.ContainerRemove(ctx, id, client.ContainerRemoveOptions{RemoveVolumes: false, Force: false})
		return err
	default:
		return errors.New("docker action is not allowed")
	}
}

func ValidateAction(id, action string) error {
	if !containerIDPattern.MatchString(id) {
		return errors.New("invalid container id")
	}
	switch action {
	case "start", "stop", "restart", "remove":
		return nil
	default:
		return errors.New("docker action is not allowed")
	}
}

func formatPorts(ports []containertypes.PortSummary) []string {
	result := make([]string, 0, len(ports))
	for _, port := range ports {
		containerPort := fmt.Sprintf("%d/%s", port.PrivatePort, port.Type)
		if port.PublicPort > 0 {
			host := port.IP.String()
			if !port.IP.IsValid() {
				host = "0.0.0.0"
			}
			containerPort = fmt.Sprintf("%s:%d→%s", host, port.PublicPort, containerPort)
		}
		result = append(result, containerPort)
	}
	return result
}
