package docker

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/netip"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	cerrdefs "github.com/containerd/errdefs"
	"github.com/moby/moby/api/pkg/stdcopy"
	containertypes "github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/mount"
	networktypes "github.com/moby/moby/api/types/network"
	"github.com/moby/moby/client"
)

var (
	containerIDPattern   = regexp.MustCompile(`^[a-f0-9]{12,64}$`)
	imageIDPattern       = regexp.MustCompile(`^(sha256:)?[a-f0-9]{12,64}$`)
	imageRefPattern      = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._/:@-]{0,254}$`)
	containerNamePattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]{0,62}$`)
	volumeNamePattern    = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]{0,127}$`)
	envNamePattern       = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]{0,127}$`)
)

var (
	ErrInvalid  = errors.New("invalid docker request")
	ErrNotFound = errors.New("docker object not found")
	ErrConflict = errors.New("docker object is in use")
)

// Host paths that must never be bind-mounted into a container from the panel.
var forbiddenBindSources = []string{"/", "/etc", "/root", "/boot", "/proc", "/sys", "/dev", "/run", "/var/run", "/var/lib/docker", "/usr", "/bin", "/sbin", "/lib"}

var containerActions = map[string]bool{"start": true, "stop": true, "restart": true, "pause": true, "unpause": true, "kill": true, "remove": true, "force-remove": true}

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

type Image struct {
	ID         string    `json:"id"`
	Tags       []string  `json:"tags"`
	Size       int64     `json:"size"`
	Containers int64     `json:"containers"`
	CreatedAt  time.Time `json:"created_at"`
}

type Volume struct {
	Name       string `json:"name"`
	Driver     string `json:"driver"`
	Mountpoint string `json:"mountpoint"`
	CreatedAt  string `json:"created_at"`
}

type Network struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Driver   string `json:"driver"`
	Scope    string `json:"scope"`
	Internal bool   `json:"internal"`
}

type PortMapping struct {
	HostIP        string `json:"host_ip"`
	HostPort      int    `json:"host_port"`
	ContainerPort int    `json:"container_port"`
	Protocol      string `json:"protocol"`
}

type VolumeMapping struct {
	Source   string `json:"source"`
	Target   string `json:"target"`
	ReadOnly bool   `json:"read_only"`
}

type RunRequest struct {
	Image         string          `json:"image"`
	Name          string          `json:"name"`
	Ports         []PortMapping   `json:"ports"`
	Env           []string        `json:"env"`
	Volumes       []VolumeMapping `json:"volumes"`
	RestartPolicy string          `json:"restart_policy"`
	Start         bool            `json:"start"`
}

type PullJob struct {
	ID         string     `json:"id"`
	Image      string     `json:"image"`
	Status     string     `json:"status"`
	Error      string     `json:"error,omitempty"`
	StartedAt  time.Time  `json:"started_at"`
	FinishedAt *time.Time `json:"finished_at,omitempty"`
}

type PruneReport struct {
	Deleted        int    `json:"deleted"`
	SpaceReclaimed uint64 `json:"space_reclaimed"`
}

type Controller interface {
	List(context.Context) ([]Container, error)
	Action(context.Context, string, string) error
	Logs(context.Context, string, int) (string, error)
	Run(context.Context, RunRequest) (string, error)
	Images(context.Context) ([]Image, error)
	StartPull(string) (PullJob, error)
	PullJobs() []PullJob
	RemoveImage(context.Context, string, bool) error
	Volumes(context.Context) ([]Volume, error)
	CreateVolume(context.Context, string) error
	RemoveVolume(context.Context, string) error
	Networks(context.Context) ([]Network, error)
	RemoveNetwork(context.Context, string) error
	Prune(context.Context, string) (PruneReport, error)
}

type engineClient interface {
	ContainerList(context.Context, client.ContainerListOptions) (client.ContainerListResult, error)
	ContainerStart(context.Context, string, client.ContainerStartOptions) (client.ContainerStartResult, error)
	ContainerStop(context.Context, string, client.ContainerStopOptions) (client.ContainerStopResult, error)
	ContainerRestart(context.Context, string, client.ContainerRestartOptions) (client.ContainerRestartResult, error)
	ContainerRemove(context.Context, string, client.ContainerRemoveOptions) (client.ContainerRemoveResult, error)
	ContainerPause(context.Context, string, client.ContainerPauseOptions) (client.ContainerPauseResult, error)
	ContainerUnpause(context.Context, string, client.ContainerUnpauseOptions) (client.ContainerUnpauseResult, error)
	ContainerKill(context.Context, string, client.ContainerKillOptions) (client.ContainerKillResult, error)
	ContainerInspect(context.Context, string, client.ContainerInspectOptions) (client.ContainerInspectResult, error)
	ContainerLogs(context.Context, string, client.ContainerLogsOptions) (client.ContainerLogsResult, error)
	ContainerCreate(context.Context, client.ContainerCreateOptions) (client.ContainerCreateResult, error)
	ContainerPrune(context.Context, client.ContainerPruneOptions) (client.ContainerPruneResult, error)
	ImageList(context.Context, client.ImageListOptions) (client.ImageListResult, error)
	ImagePull(context.Context, string, client.ImagePullOptions) (client.ImagePullResponse, error)
	ImageRemove(context.Context, string, client.ImageRemoveOptions) (client.ImageRemoveResult, error)
	ImagePrune(context.Context, client.ImagePruneOptions) (client.ImagePruneResult, error)
	VolumeList(context.Context, client.VolumeListOptions) (client.VolumeListResult, error)
	VolumeCreate(context.Context, client.VolumeCreateOptions) (client.VolumeCreateResult, error)
	VolumeRemove(context.Context, string, client.VolumeRemoveOptions) (client.VolumeRemoveResult, error)
	VolumePrune(context.Context, client.VolumePruneOptions) (client.VolumePruneResult, error)
	NetworkList(context.Context, client.NetworkListOptions) (client.NetworkListResult, error)
	NetworkRemove(context.Context, string, client.NetworkRemoveOptions) (client.NetworkRemoveResult, error)
	NetworkPrune(context.Context, client.NetworkPruneOptions) (client.NetworkPruneResult, error)
}

type pullRegistry struct {
	mu   sync.Mutex
	seq  int
	jobs []PullJob
}

type Manager struct {
	client engineClient
	pulls  *pullRegistry
}

func NewManager() (*Manager, error) {
	apiClient, err := client.New(client.WithHost("unix:///var/run/docker.sock"))
	if err != nil {
		return nil, err
	}
	return NewManagerWithClient(apiClient), nil
}

func NewManagerWithClient(apiClient engineClient) *Manager {
	return &Manager{client: apiClient, pulls: &pullRegistry{}}
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
	timeout := 15
	var err error
	switch action {
	case "start":
		_, err = m.client.ContainerStart(ctx, id, client.ContainerStartOptions{})
	case "stop":
		_, err = m.client.ContainerStop(ctx, id, client.ContainerStopOptions{Timeout: &timeout})
	case "restart":
		_, err = m.client.ContainerRestart(ctx, id, client.ContainerRestartOptions{Timeout: &timeout})
	case "pause":
		_, err = m.client.ContainerPause(ctx, id, client.ContainerPauseOptions{})
	case "unpause":
		_, err = m.client.ContainerUnpause(ctx, id, client.ContainerUnpauseOptions{})
	case "kill":
		_, err = m.client.ContainerKill(ctx, id, client.ContainerKillOptions{})
	case "remove":
		_, err = m.client.ContainerRemove(ctx, id, client.ContainerRemoveOptions{RemoveVolumes: false, Force: false})
	case "force-remove":
		_, err = m.client.ContainerRemove(ctx, id, client.ContainerRemoveOptions{RemoveVolumes: false, Force: true})
	}
	return mapError(err)
}

func ValidateAction(id, action string) error {
	if !containerIDPattern.MatchString(id) {
		return errors.New("invalid container id")
	}
	if !containerActions[action] {
		return errors.New("docker action is not allowed")
	}
	return nil
}

const maxLogBytes = 1 << 20

// Logs returns the last tail lines of stdout/stderr, demultiplexed for non-TTY containers.
func (m Manager) Logs(ctx context.Context, id string, tail int) (string, error) {
	if !containerIDPattern.MatchString(id) || tail < 1 || tail > 5000 {
		return "", ErrInvalid
	}
	inspect, err := m.client.ContainerInspect(ctx, id, client.ContainerInspectOptions{})
	if err != nil {
		return "", mapError(err)
	}
	reader, err := m.client.ContainerLogs(ctx, id, client.ContainerLogsOptions{ShowStdout: true, ShowStderr: true, Timestamps: true, Tail: strconv.Itoa(tail)})
	if err != nil {
		return "", mapError(err)
	}
	defer reader.Close()
	limited := io.LimitReader(reader, maxLogBytes)
	var output bytes.Buffer
	if inspect.Container.Config != nil && inspect.Container.Config.Tty {
		_, err = io.Copy(&output, limited)
	} else {
		_, err = stdcopy.StdCopy(&output, &output, limited)
	}
	if err != nil && !errors.Is(err, io.EOF) {
		return "", err
	}
	return strings.ToValidUTF8(output.String(), "�"), nil
}

func ValidateRunRequest(request RunRequest) error {
	if !validImageRef(request.Image) {
		return fmt.Errorf("%w: image", ErrInvalid)
	}
	if request.Name != "" && !containerNamePattern.MatchString(request.Name) {
		return fmt.Errorf("%w: name", ErrInvalid)
	}
	switch request.RestartPolicy {
	case "", "no", "always", "on-failure", "unless-stopped":
	default:
		return fmt.Errorf("%w: restart policy", ErrInvalid)
	}
	if len(request.Ports) > 32 || len(request.Env) > 64 || len(request.Volumes) > 16 {
		return fmt.Errorf("%w: too many entries", ErrInvalid)
	}
	seenHostPorts := map[string]bool{}
	for _, port := range request.Ports {
		if port.ContainerPort < 1 || port.ContainerPort > 65535 || port.HostPort < 0 || port.HostPort > 65535 {
			return fmt.Errorf("%w: port", ErrInvalid)
		}
		if port.Protocol != "tcp" && port.Protocol != "udp" {
			return fmt.Errorf("%w: protocol", ErrInvalid)
		}
		if port.HostIP != "" {
			if _, err := netip.ParseAddr(port.HostIP); err != nil {
				return fmt.Errorf("%w: host ip", ErrInvalid)
			}
		}
		if port.HostPort > 0 {
			key := fmt.Sprintf("%s/%d/%s", port.HostIP, port.HostPort, port.Protocol)
			if seenHostPorts[key] {
				return fmt.Errorf("%w: duplicate host port", ErrInvalid)
			}
			seenHostPorts[key] = true
		}
	}
	for _, entry := range request.Env {
		name, value, found := strings.Cut(entry, "=")
		if !found || !envNamePattern.MatchString(name) || len(value) > 4096 || strings.ContainsAny(value, "\x00\n\r") {
			return fmt.Errorf("%w: env", ErrInvalid)
		}
	}
	for _, volume := range request.Volumes {
		if !path.IsAbs(volume.Target) || path.Clean(volume.Target) != volume.Target || volume.Target == "/" || strings.ContainsAny(volume.Target, ":,\x00") {
			return fmt.Errorf("%w: volume target", ErrInvalid)
		}
		if strings.HasPrefix(volume.Source, "/") {
			if err := validateBindSource(volume.Source); err != nil {
				return err
			}
		} else if !volumeNamePattern.MatchString(volume.Source) {
			return fmt.Errorf("%w: volume source", ErrInvalid)
		}
	}
	return nil
}

func validateBindSource(source string) error {
	if path.Clean(source) != source || strings.ContainsAny(source, ":,\x00") {
		return fmt.Errorf("%w: bind source", ErrInvalid)
	}
	for _, forbidden := range forbiddenBindSources {
		if source == forbidden || (forbidden != "/" && strings.HasPrefix(source, forbidden+"/")) {
			return fmt.Errorf("%w: bind source is protected", ErrInvalid)
		}
	}
	if strings.HasSuffix(source, ".sock") {
		return fmt.Errorf("%w: bind source is protected", ErrInvalid)
	}
	return nil
}

func validImageRef(ref string) bool {
	return imageRefPattern.MatchString(ref) && !strings.Contains(ref, "..") && !strings.Contains(ref, "//")
}

// Run creates (and optionally starts) a container without privileged mode, host namespaces or extra capabilities.
func (m Manager) Run(ctx context.Context, request RunRequest) (string, error) {
	if err := ValidateRunRequest(request); err != nil {
		return "", err
	}
	exposed := networktypes.PortSet{}
	bindings := networktypes.PortMap{}
	for _, mapping := range request.Ports {
		port, err := networktypes.ParsePort(fmt.Sprintf("%d/%s", mapping.ContainerPort, mapping.Protocol))
		if err != nil {
			return "", fmt.Errorf("%w: port", ErrInvalid)
		}
		exposed[port] = struct{}{}
		binding := networktypes.PortBinding{}
		if mapping.HostPort > 0 {
			binding.HostPort = strconv.Itoa(mapping.HostPort)
		}
		if mapping.HostIP != "" {
			binding.HostIP = netip.MustParseAddr(mapping.HostIP)
		}
		bindings[port] = append(bindings[port], binding)
	}
	mounts := make([]mount.Mount, 0, len(request.Volumes))
	for _, volume := range request.Volumes {
		mountType := mount.TypeVolume
		if strings.HasPrefix(volume.Source, "/") {
			mountType = mount.TypeBind
		}
		mounts = append(mounts, mount.Mount{Type: mountType, Source: volume.Source, Target: volume.Target, ReadOnly: volume.ReadOnly})
	}
	policy := request.RestartPolicy
	if policy == "" {
		policy = "no"
	}
	created, err := m.client.ContainerCreate(ctx, client.ContainerCreateOptions{
		Name:   request.Name,
		Config: &containertypes.Config{Image: request.Image, Env: request.Env, ExposedPorts: exposed},
		HostConfig: &containertypes.HostConfig{
			PortBindings:  bindings,
			Mounts:        mounts,
			RestartPolicy: containertypes.RestartPolicy{Name: containertypes.RestartPolicyMode(policy)},
			Privileged:    false,
		},
	})
	if err != nil {
		return "", mapError(err)
	}
	if request.Start {
		if _, err := m.client.ContainerStart(ctx, created.ID, client.ContainerStartOptions{}); err != nil {
			return created.ID, mapError(err)
		}
	}
	return created.ID, nil
}

func (m Manager) Images(ctx context.Context) ([]Image, error) {
	result, err := m.client.ImageList(ctx, client.ImageListOptions{All: false})
	if err != nil {
		return nil, err
	}
	items := make([]Image, 0, len(result.Items))
	for _, summary := range result.Items {
		tags := make([]string, 0, len(summary.RepoTags))
		for _, tag := range summary.RepoTags {
			if tag != "<none>:<none>" {
				tags = append(tags, tag)
			}
		}
		items = append(items, Image{ID: summary.ID, Tags: tags, Size: summary.Size, Containers: summary.Containers, CreatedAt: time.Unix(summary.Created, 0).UTC()})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].CreatedAt.After(items[j].CreatedAt) })
	return items, nil
}

// StartPull pulls an image in the background because registry downloads can outlive HTTP timeouts.
func (m Manager) StartPull(ref string) (PullJob, error) {
	if !validImageRef(ref) {
		return PullJob{}, ErrInvalid
	}
	if !strings.Contains(path.Base(ref), ":") && !strings.Contains(ref, "@") {
		ref += ":latest"
	}
	m.pulls.mu.Lock()
	for _, job := range m.pulls.jobs {
		if job.Image == ref && job.Status == "running" {
			m.pulls.mu.Unlock()
			return job, nil
		}
	}
	m.pulls.seq++
	job := PullJob{ID: strconv.Itoa(m.pulls.seq), Image: ref, Status: "running", StartedAt: time.Now().UTC()}
	m.pulls.jobs = append(m.pulls.jobs, job)
	if len(m.pulls.jobs) > 20 {
		m.pulls.jobs = m.pulls.jobs[len(m.pulls.jobs)-20:]
	}
	m.pulls.mu.Unlock()

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
		defer cancel()
		err := m.pull(ctx, ref)
		m.pulls.mu.Lock()
		defer m.pulls.mu.Unlock()
		for index := range m.pulls.jobs {
			if m.pulls.jobs[index].ID != job.ID {
				continue
			}
			now := time.Now().UTC()
			m.pulls.jobs[index].FinishedAt = &now
			m.pulls.jobs[index].Status = "done"
			if err != nil {
				m.pulls.jobs[index].Status = "failed"
				m.pulls.jobs[index].Error = truncate(err.Error(), 300)
			}
		}
	}()
	return job, nil
}

func (m Manager) pull(ctx context.Context, ref string) error {
	response, err := m.client.ImagePull(ctx, ref, client.ImagePullOptions{})
	if err != nil {
		return err
	}
	for message, err := range response.JSONMessages(ctx) {
		if err != nil {
			return err
		}
		if message.Error != nil {
			return errors.New(message.Error.Message)
		}
	}
	return nil
}

func (m Manager) PullJobs() []PullJob {
	m.pulls.mu.Lock()
	defer m.pulls.mu.Unlock()
	jobs := make([]PullJob, len(m.pulls.jobs))
	for index, job := range m.pulls.jobs {
		jobs[len(jobs)-1-index] = job
	}
	return jobs
}

func (m Manager) RemoveImage(ctx context.Context, id string, force bool) error {
	if !imageIDPattern.MatchString(id) && !validImageRef(id) {
		return ErrInvalid
	}
	_, err := m.client.ImageRemove(ctx, id, client.ImageRemoveOptions{Force: force, PruneChildren: true})
	return mapError(err)
}

func (m Manager) Volumes(ctx context.Context) ([]Volume, error) {
	result, err := m.client.VolumeList(ctx, client.VolumeListOptions{})
	if err != nil {
		return nil, err
	}
	items := make([]Volume, 0, len(result.Items))
	for _, volume := range result.Items {
		items = append(items, Volume{Name: volume.Name, Driver: volume.Driver, Mountpoint: volume.Mountpoint, CreatedAt: volume.CreatedAt})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Name < items[j].Name })
	return items, nil
}

func (m Manager) CreateVolume(ctx context.Context, name string) error {
	if !volumeNamePattern.MatchString(name) {
		return ErrInvalid
	}
	_, err := m.client.VolumeCreate(ctx, client.VolumeCreateOptions{Name: name})
	return mapError(err)
}

func (m Manager) RemoveVolume(ctx context.Context, name string) error {
	if !volumeNamePattern.MatchString(name) {
		return ErrInvalid
	}
	_, err := m.client.VolumeRemove(ctx, name, client.VolumeRemoveOptions{Force: false})
	return mapError(err)
}

func (m Manager) Networks(ctx context.Context) ([]Network, error) {
	result, err := m.client.NetworkList(ctx, client.NetworkListOptions{})
	if err != nil {
		return nil, err
	}
	items := make([]Network, 0, len(result.Items))
	for _, network := range result.Items {
		items = append(items, Network{ID: network.ID, Name: network.Name, Driver: network.Driver, Scope: network.Scope, Internal: network.Internal})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Name < items[j].Name })
	return items, nil
}

func (m Manager) RemoveNetwork(ctx context.Context, id string) error {
	if !containerIDPattern.MatchString(id) {
		return ErrInvalid
	}
	_, err := m.client.NetworkRemove(ctx, id, client.NetworkRemoveOptions{})
	return mapError(err)
}

// Prune removes stopped containers, dangling images, unused anonymous volumes or unused networks.
func (m Manager) Prune(ctx context.Context, kind string) (PruneReport, error) {
	switch kind {
	case "containers":
		result, err := m.client.ContainerPrune(ctx, client.ContainerPruneOptions{})
		return PruneReport{Deleted: len(result.Report.ContainersDeleted), SpaceReclaimed: result.Report.SpaceReclaimed}, err
	case "images":
		result, err := m.client.ImagePrune(ctx, client.ImagePruneOptions{})
		return PruneReport{Deleted: len(result.Report.ImagesDeleted), SpaceReclaimed: result.Report.SpaceReclaimed}, err
	case "volumes":
		result, err := m.client.VolumePrune(ctx, client.VolumePruneOptions{})
		return PruneReport{Deleted: len(result.Report.VolumesDeleted), SpaceReclaimed: result.Report.SpaceReclaimed}, err
	case "networks":
		result, err := m.client.NetworkPrune(ctx, client.NetworkPruneOptions{})
		return PruneReport{Deleted: len(result.Report.NetworksDeleted)}, err
	default:
		return PruneReport{}, ErrInvalid
	}
}

func mapError(err error) error {
	switch {
	case err == nil:
		return nil
	case cerrdefs.IsNotFound(err):
		return fmt.Errorf("%w: %s", ErrNotFound, truncate(err.Error(), 300))
	case cerrdefs.IsConflict(err):
		return fmt.Errorf("%w: %s", ErrConflict, truncate(err.Error(), 300))
	default:
		return err
	}
}

func truncate(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	return value[:limit]
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
