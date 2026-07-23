package firewall

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

const maxRequestBytes = 4096

var numberedRulePattern = regexp.MustCompile(`^\[\s*(\d+)\]\s+(.+?)\s{2,}(ALLOW|DENY|REJECT)\s+(IN|OUT)\s+(.+)$`)

type Rule struct {
	Number    int    `json:"number"`
	To        string `json:"to"`
	Action    string `json:"action"`
	Direction string `json:"direction"`
	From      string `json:"from"`
}

type Status struct {
	Active bool   `json:"active"`
	Rules  []Rule `json:"rules"`
}

type AddRequest struct {
	Action   string `json:"action"`
	Port     int    `json:"port"`
	Protocol string `json:"protocol"`
	Source   string `json:"source"`
}

type Controller interface {
	Status(context.Context) (Status, error)
	Add(context.Context, AddRequest) error
	Delete(context.Context, int) error
}

type Manager struct {
	Executable string
}

func NewManager() (*Manager, error) {
	executable, err := os.Executable()
	if err != nil {
		return nil, err
	}
	return &Manager{Executable: executable}, nil
}

func (m Manager) Status(ctx context.Context) (Status, error) {
	var status Status
	if err := m.run(ctx, privilegedRequest{Operation: "status"}, &status); err != nil {
		return Status{}, err
	}
	return status, nil
}

func (m Manager) Add(ctx context.Context, request AddRequest) error {
	return m.run(ctx, privilegedRequest{Operation: "add", Rule: &request}, nil)
}

func (m Manager) Delete(ctx context.Context, number int) error {
	return m.run(ctx, privilegedRequest{Operation: "delete", Number: number}, nil)
}

func (m Manager) run(ctx context.Context, request privilegedRequest, target any) error {
	if err := validateRequest(request); err != nil {
		return err
	}
	payload, err := json.Marshal(request)
	if err != nil {
		return err
	}
	command := exec.CommandContext(ctx, "/usr/bin/sudo", "-n", m.Executable, "privileged-firewall")
	command.Stdin = bytes.NewReader(payload)
	output, err := command.CombinedOutput()
	if err != nil {
		return fmt.Errorf("privileged firewall operation failed: %w: %s", err, strings.TrimSpace(string(output)))
	}
	if target != nil && json.Unmarshal(output, target) != nil {
		return errors.New("invalid firewall helper response")
	}
	return nil
}

type privilegedRequest struct {
	Operation string      `json:"operation"`
	Rule      *AddRequest `json:"rule,omitempty"`
	Number    int         `json:"number,omitempty"`
}

func RunPrivileged(input io.Reader, output io.Writer) error {
	if os.Geteuid() != 0 {
		return errors.New("privileged-firewall must run as root")
	}
	decoder := json.NewDecoder(io.LimitReader(input, maxRequestBytes))
	decoder.DisallowUnknownFields()
	var request privilegedRequest
	if err := decoder.Decode(&request); err != nil {
		return errors.New("invalid firewall request")
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return errors.New("invalid firewall request")
	}
	if err := validateRequest(request); err != nil {
		return err
	}
	switch request.Operation {
	case "status":
		command := exec.Command("/usr/bin/env", "LC_ALL=C", "/usr/sbin/ufw", "status", "numbered")
		data, err := command.CombinedOutput()
		if err != nil {
			return fmt.Errorf("ufw status failed: %w", err)
		}
		return json.NewEncoder(output).Encode(parseStatus(string(data)))
	case "add":
		rule := request.Rule
		args := []string{"--force", rule.Action, "from", normalizeSource(rule.Source), "to", "any", "port", strconv.Itoa(rule.Port), "proto", rule.Protocol}
		if data, err := exec.Command("/usr/sbin/ufw", args...).CombinedOutput(); err != nil {
			return fmt.Errorf("ufw add failed: %w: %s", err, strings.TrimSpace(string(data)))
		}
	case "delete":
		if data, err := exec.Command("/usr/sbin/ufw", "--force", "delete", strconv.Itoa(request.Number)).CombinedOutput(); err != nil {
			return fmt.Errorf("ufw delete failed: %w: %s", err, strings.TrimSpace(string(data)))
		}
	}
	return nil
}

func validateRequest(request privilegedRequest) error {
	switch request.Operation {
	case "status":
		if request.Rule != nil || request.Number != 0 {
			return errors.New("invalid status request")
		}
		return nil
	case "add":
		if request.Rule == nil || request.Number != 0 {
			return errors.New("invalid add request")
		}
		return ValidateRule(*request.Rule)
	case "delete":
		if request.Rule != nil || request.Number < 1 || request.Number > 10000 {
			return errors.New("invalid delete request")
		}
		return nil
	default:
		return errors.New("firewall operation is not allowed")
	}
}

func ValidateRule(rule AddRequest) error {
	if rule.Action != "allow" && rule.Action != "deny" {
		return errors.New("firewall action is not allowed")
	}
	if rule.Port < 1 || rule.Port > 65535 {
		return errors.New("invalid firewall port")
	}
	if rule.Action == "deny" && rule.Port == 22 {
		return errors.New("default SSH port is protected")
	}
	if rule.Protocol != "tcp" && rule.Protocol != "udp" {
		return errors.New("invalid firewall protocol")
	}
	source := normalizeSource(rule.Source)
	if source == "any" {
		return nil
	}
	if net.ParseIP(source) != nil {
		return nil
	}
	if _, _, err := net.ParseCIDR(source); err != nil {
		return errors.New("invalid firewall source")
	}
	return nil
}

func normalizeSource(source string) string {
	source = strings.TrimSpace(strings.ToLower(source))
	if source == "" || source == "any" || source == "anywhere" {
		return "any"
	}
	return source
}

func parseStatus(value string) Status {
	status := Status{Rules: make([]Rule, 0)}
	scanner := bufio.NewScanner(strings.NewReader(value))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "Status: active" {
			status.Active = true
		}
		matches := numberedRulePattern.FindStringSubmatch(line)
		if len(matches) != 6 {
			continue
		}
		number, _ := strconv.Atoi(matches[1])
		status.Rules = append(status.Rules, Rule{Number: number, To: strings.TrimSpace(matches[2]), Action: strings.ToLower(matches[3]), Direction: strings.ToLower(matches[4]), From: strings.TrimSpace(matches[5])})
	}
	return status
}
