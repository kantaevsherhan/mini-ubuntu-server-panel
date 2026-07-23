package services

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strings"
)

const maxActionRequestBytes = 2048

var unitPattern = regexp.MustCompile(`^[A-Za-z0-9_.@:-]{1,240}\.service$`)

type Service struct {
	Name        string `json:"name"`
	LoadState   string `json:"load_state"`
	ActiveState string `json:"active_state"`
	SubState    string `json:"sub_state"`
	Enabled     string `json:"enabled"`
	Description string `json:"description"`
}

type Controller interface {
	List(context.Context) ([]Service, error)
	Action(context.Context, string, string) error
}

type Manager struct {
	Executable string
	Systemctl  string
}

func NewManager() (*Manager, error) {
	executable, err := os.Executable()
	if err != nil {
		return nil, err
	}
	return &Manager{Executable: executable, Systemctl: "/usr/bin/systemctl"}, nil
}

func (m Manager) List(ctx context.Context) ([]Service, error) {
	unitsOutput, err := exec.CommandContext(ctx, m.Systemctl, "list-units", "--all", "--type=service", "--plain", "--no-legend", "--no-pager").Output()
	if err != nil {
		return nil, fmt.Errorf("list systemd units: %w", err)
	}
	filesOutput, err := exec.CommandContext(ctx, m.Systemctl, "list-unit-files", "--type=service", "--no-legend", "--no-pager").Output()
	if err != nil {
		return nil, fmt.Errorf("list systemd unit files: %w", err)
	}
	return parseUnits(string(unitsOutput), string(filesOutput)), nil
}

func (m Manager) Action(ctx context.Context, unit, action string) error {
	request := actionRequest{Unit: unit, Action: action}
	if err := ValidateAction(request.Unit, request.Action); err != nil {
		return err
	}
	payload, err := json.Marshal(request)
	if err != nil {
		return err
	}
	command := exec.CommandContext(ctx, "/usr/bin/sudo", "-n", m.Executable, "privileged-service")
	command.Stdin = strings.NewReader(string(payload))
	if output, err := command.CombinedOutput(); err != nil {
		return fmt.Errorf("privileged service action failed: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}

type actionRequest struct {
	Unit   string `json:"unit"`
	Action string `json:"action"`
}

func RunPrivilegedAction(input io.Reader) error {
	if os.Geteuid() != 0 {
		return errors.New("privileged-service must run as root")
	}
	decoder := json.NewDecoder(io.LimitReader(input, maxActionRequestBytes))
	decoder.DisallowUnknownFields()
	var request actionRequest
	if err := decoder.Decode(&request); err != nil {
		return errors.New("invalid service action request")
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return errors.New("invalid service action request")
	}
	if err := ValidateAction(request.Unit, request.Action); err != nil {
		return err
	}
	command := exec.Command("/usr/bin/systemctl", request.Action, "--", request.Unit)
	if output, err := command.CombinedOutput(); err != nil {
		return fmt.Errorf("systemctl action failed: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}

func ValidateAction(unit, action string) error {
	if !unitPattern.MatchString(unit) {
		return errors.New("invalid service unit")
	}
	if unit == "mini-ubuntu-server.service" {
		return errors.New("panel service is protected")
	}
	switch action {
	case "start", "stop", "restart", "enable", "disable":
		return nil
	default:
		return errors.New("service action is not allowed")
	}
}

func parseUnits(unitsOutput, filesOutput string) []Service {
	enabled := make(map[string]string)
	scanner := bufio.NewScanner(strings.NewReader(filesOutput))
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) >= 2 && unitPattern.MatchString(fields[0]) {
			enabled[fields[0]] = fields[1]
		}
	}
	items := make([]Service, 0, len(enabled))
	seen := make(map[string]bool)
	scanner = bufio.NewScanner(strings.NewReader(unitsOutput))
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 5 || !unitPattern.MatchString(fields[0]) {
			continue
		}
		items = append(items, Service{Name: fields[0], LoadState: fields[1], ActiveState: fields[2], SubState: fields[3], Enabled: enabled[fields[0]], Description: strings.Join(fields[4:], " ")})
		seen[fields[0]] = true
	}
	for name, state := range enabled {
		if !seen[name] {
			items = append(items, Service{Name: name, LoadState: "unloaded", ActiveState: "inactive", SubState: "dead", Enabled: state})
		}
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Name < items[j].Name })
	return items
}
