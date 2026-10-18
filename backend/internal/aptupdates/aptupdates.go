// Package aptupdates lists pending apt upgrades and runs refresh/upgrade jobs.
//
// Listing uses an unprivileged apt-get simulation. Mutations go through the exact
// sudoers helper "privileged-apt", which starts apt in a transient systemd unit:
// the panel service runs with ProtectSystem=strict, so apt must not inherit its
// mount namespace, and a transient unit also survives a panel restart mid-upgrade.
package aptupdates

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	UnitName        = "mini-ubuntu-server-apt"
	maxRequestBytes = 16 * 1024
	maxPackages     = 500
	maxLogLines     = 400
	maxLogLineBytes = 512
)

var (
	ErrBusy           = errors.New("an apt job is already running")
	ErrInvalidRequest = errors.New("invalid apt request")
	// Debian policy: lowercase alphanumerics plus + - . (optional :arch qualifier).
	packagePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9+.-]{0,127}(:[a-z0-9-]{1,16})?$`)
	// "Inst name [current] (candidate origins [arch])"; [current] is absent for new packages.
	instPattern = regexp.MustCompile(`^Inst (\S+) (?:\[([^\]]+)\] )?\((\S+) ([^\[]*?)\s*(?:\[[^\]]+\])?\)`)
)

type Package struct {
	Name      string `json:"name"`
	Current   string `json:"current"`
	Candidate string `json:"candidate"`
	Origin    string `json:"origin"`
	Security  bool   `json:"security"`
	New       bool   `json:"new"`
}

type AutoUpgrades struct {
	Installed bool `json:"installed"`
	Enabled   bool `json:"enabled"`
}

type Job struct {
	State      string     `json:"state"` // idle, running, succeeded, failed
	Operation  string     `json:"operation,omitempty"`
	StartedAt  *time.Time `json:"started_at,omitempty"`
	FinishedAt *time.Time `json:"finished_at,omitempty"`
	ExitCode   *int       `json:"exit_code,omitempty"`
	Log        []string   `json:"log"`
}

type Status struct {
	Packages       []Package    `json:"packages"`
	SecurityCount  int          `json:"security_count"`
	LastRefresh    *time.Time   `json:"last_refresh,omitempty"`
	RebootRequired bool         `json:"reboot_required"`
	RebootPackages []string     `json:"reboot_packages"`
	AutoUpgrades   AutoUpgrades `json:"auto_upgrades"`
	Job            Job          `json:"job"`
}

// StartRequest starts "refresh" (apt-get update) or "upgrade". An upgrade with no
// packages upgrades everything; otherwise only the listed upgradable packages.
type StartRequest struct {
	Operation string   `json:"operation"`
	Packages  []string `json:"packages,omitempty"`
}

type Controller interface {
	Status(context.Context) (Status, error)
	Job(context.Context) (Job, error)
	Start(context.Context, StartRequest) error
}

type Manager struct {
	Executable string
	Root       string
}

func NewManager() (*Manager, error) {
	executable, err := os.Executable()
	if err != nil {
		return nil, err
	}
	return &Manager{Executable: executable, Root: "/"}, nil
}

func (m Manager) path(parts ...string) string {
	return filepath.Join(append([]string{m.Root}, parts...)...)
}

func (m Manager) Status(ctx context.Context) (Status, error) {
	packages, err := simulate(ctx)
	if err != nil {
		return Status{}, err
	}
	job, err := m.Job(ctx)
	if err != nil {
		return Status{}, err
	}
	status := Status{Packages: packages, Job: job, RebootPackages: []string{}}
	for _, pkg := range packages {
		if pkg.Security {
			status.SecurityCount++
		}
	}
	status.LastRefresh = m.lastRefresh()
	if _, err := os.Stat(m.path("var", "run", "reboot-required")); err == nil {
		status.RebootRequired = true
		status.RebootPackages = readLines(m.path("var", "run", "reboot-required.pkgs"), 100)
	}
	status.AutoUpgrades = m.autoUpgrades()
	return status, nil
}

func (m Manager) Job(ctx context.Context) (Job, error) {
	var job Job
	if err := m.run(ctx, privilegedRequest{Operation: "status"}, &job); err != nil {
		return Job{}, err
	}
	return job, nil
}

func (m Manager) Start(ctx context.Context, request StartRequest) error {
	return m.run(ctx, privilegedRequest(request), nil)
}

func (m Manager) run(ctx context.Context, request privilegedRequest, target any) error {
	if err := validate(request); err != nil {
		return err
	}
	payload, err := json.Marshal(request)
	if err != nil {
		return err
	}
	command := exec.CommandContext(ctx, "/usr/bin/sudo", "-n", m.Executable, "privileged-apt")
	command.Stdin = bytes.NewReader(payload)
	var stdout, stderr bytes.Buffer
	command.Stdout, command.Stderr = &stdout, &stderr
	if err := command.Run(); err != nil {
		if strings.Contains(stderr.String(), ErrBusy.Error()) {
			return ErrBusy
		}
		return fmt.Errorf("privileged apt operation failed: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	if target != nil && json.Unmarshal(stdout.Bytes(), target) != nil {
		return errors.New("invalid apt helper response")
	}
	return nil
}

func (m Manager) lastRefresh() *time.Time {
	// Touched by APT::Update::Post-Invoke-Success on Ubuntu; the lists directory is a fallback.
	for _, candidate := range []string{m.path("var", "lib", "apt", "periodic", "update-success-stamp"), m.path("var", "lib", "apt", "lists")} {
		if info, err := os.Stat(candidate); err == nil {
			at := info.ModTime().UTC()
			return &at
		}
	}
	return nil
}

func (m Manager) autoUpgrades() AutoUpgrades {
	result := AutoUpgrades{}
	if _, err := os.Stat(m.path("usr", "bin", "unattended-upgrade")); err == nil {
		result.Installed = true
	}
	data, err := os.ReadFile(m.path("etc", "apt", "apt.conf.d", "20auto-upgrades"))
	if err == nil {
		result.Enabled = ParseAutoUpgrades(string(data))
	}
	return result
}

// ParseAutoUpgrades reports whether APT::Periodic::Unattended-Upgrade is non-zero.
func ParseAutoUpgrades(config string) bool {
	pattern := regexp.MustCompile(`(?m)^\s*APT::Periodic::Unattended-Upgrade\s+"(\d+)"\s*;`)
	match := pattern.FindStringSubmatch(config)
	return match != nil && match[1] != "0"
}

func simulate(ctx context.Context) ([]Package, error) {
	ctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	// Simulation reads the package lists without locks or root privileges.
	command := exec.CommandContext(ctx, "/usr/bin/apt-get", "-s", "-q", "-o", "Debug::NoLocking=true", "upgrade", "--with-new-pkgs")
	command.Env = []string{"LC_ALL=C", "PATH=/usr/sbin:/usr/bin:/sbin:/bin"}
	output, err := command.Output()
	if err != nil {
		return nil, fmt.Errorf("apt-get simulation failed: %w", err)
	}
	return ParseSimulation(string(output)), nil
}

// ParseSimulation extracts packages from "apt-get -s upgrade" output.
func ParseSimulation(output string) []Package {
	packages := make([]Package, 0)
	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		match := instPattern.FindStringSubmatch(scanner.Text())
		if match == nil {
			continue
		}
		origin := strings.TrimSpace(match[4])
		packages = append(packages, Package{
			Name:      match[1],
			Current:   match[2],
			Candidate: match[3],
			Origin:    origin,
			Security:  strings.Contains(origin, "-security"),
			New:       match[2] == "",
		})
	}
	sort.SliceStable(packages, func(i, j int) bool {
		if packages[i].Security != packages[j].Security {
			return packages[i].Security
		}
		return packages[i].Name < packages[j].Name
	})
	return packages
}

func readLines(path string, limit int) []string {
	data, err := os.ReadFile(path)
	if err != nil {
		return []string{}
	}
	lines := make([]string, 0)
	seen := map[string]bool{}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line != "" && !seen[line] && len(lines) < limit {
			seen[line] = true
			lines = append(lines, line)
		}
	}
	return lines
}

type privilegedRequest struct {
	Operation string   `json:"operation"`
	Packages  []string `json:"packages,omitempty"`
}

func validate(request privilegedRequest) error {
	switch request.Operation {
	case "status", "refresh":
		if len(request.Packages) != 0 {
			return ErrInvalidRequest
		}
	case "upgrade":
		if len(request.Packages) > maxPackages {
			return ErrInvalidRequest
		}
		seen := map[string]bool{}
		for _, name := range request.Packages {
			if !packagePattern.MatchString(name) || seen[name] {
				return ErrInvalidRequest
			}
			seen[name] = true
		}
	default:
		return ErrInvalidRequest
	}
	return nil
}

// RunPrivileged is the root side of the helper. It accepts exactly one JSON request.
func RunPrivileged(input io.Reader, output io.Writer) error {
	if os.Geteuid() != 0 {
		return errors.New("privileged-apt must run as root")
	}
	decoder := json.NewDecoder(io.LimitReader(input, maxRequestBytes))
	decoder.DisallowUnknownFields()
	var request privilegedRequest
	if err := decoder.Decode(&request); err != nil {
		return ErrInvalidRequest
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return ErrInvalidRequest
	}
	if err := validate(request); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	job, err := currentJob(ctx)
	if err != nil {
		return err
	}
	if request.Operation == "status" {
		return json.NewEncoder(output).Encode(job)
	}
	if job.State == "running" {
		return ErrBusy
	}
	args := []string{"-q"}
	description := "mini-ubuntu-server apt refresh"
	if request.Operation == "upgrade" {
		description = "mini-ubuntu-server apt upgrade"
		args = append(args, "-y", "-o", "DPkg::Lock::Timeout=120", "-o", "Dpkg::Options::=--force-confdef", "-o", "Dpkg::Options::=--force-confold")
		if len(request.Packages) == 0 {
			args = append(args, "upgrade", "--with-new-pkgs")
		} else {
			// Re-check as root that every requested name is really upgradable.
			upgradable, err := simulate(ctx)
			if err != nil {
				return err
			}
			allowed := map[string]bool{}
			for _, pkg := range upgradable {
				allowed[pkg.Name] = true
			}
			for _, name := range request.Packages {
				if !allowed[name] {
					return fmt.Errorf("package %s is not upgradable", name)
				}
			}
			args = append(args, "install", "--only-upgrade")
			args = append(args, request.Packages...)
		}
	} else {
		args = append(args, "-o", "DPkg::Lock::Timeout=120", "update")
	}
	// A finished unit stays loaded (RemainAfterExit) so its result can be read; clear it first.
	_ = exec.CommandContext(ctx, "/usr/bin/systemctl", "stop", UnitName).Run()
	_ = exec.CommandContext(ctx, "/usr/bin/systemctl", "reset-failed", UnitName).Run()
	runArgs := []string{
		"--unit=" + UnitName, "--description=" + description, "--quiet",
		"--property=RemainAfterExit=yes",
		"--setenv=DEBIAN_FRONTEND=noninteractive", "--setenv=NEEDRESTART_MODE=a", "--setenv=LC_ALL=C",
		"/usr/bin/apt-get",
	}
	runArgs = append(runArgs, args...)
	if data, err := exec.CommandContext(ctx, "/usr/bin/systemd-run", runArgs...).CombinedOutput(); err != nil {
		return fmt.Errorf("systemd-run failed: %w: %s", err, strings.TrimSpace(string(data)))
	}
	return nil
}

func currentJob(ctx context.Context) (Job, error) {
	properties := "LoadState,ActiveState,SubState,Result,ExecMainStatus,ExecMainStartTimestamp,ExecMainExitTimestamp,Description,InvocationID"
	data, err := exec.CommandContext(ctx, "/usr/bin/systemctl", "show", UnitName, "--timestamp=unix", "-p", properties).Output()
	if err != nil {
		// systemd < 248 (Ubuntu 20.04) has no --timestamp; ParseUnit also reads the default format.
		data, err = exec.CommandContext(ctx, "/usr/bin/systemctl", "show", UnitName, "-p", properties).Output()
	}
	if err != nil {
		return Job{}, fmt.Errorf("systemctl show failed: %w", err)
	}
	job, invocation := ParseUnit(string(data))
	job.Log = []string{}
	if invocation != "" {
		logData, err := exec.CommandContext(ctx, "/usr/bin/journalctl", "_SYSTEMD_INVOCATION_ID="+invocation, "-o", "cat", "--no-pager", "-q", "-n", strconv.Itoa(maxLogLines)).Output()
		if err == nil {
			job.Log = trimLog(string(logData))
		}
	}
	return job, nil
}

// ParseUnit maps "systemctl show" properties to a job state and returns the invocation ID.
func ParseUnit(output string) (Job, string) {
	properties := map[string]string{}
	for _, line := range strings.Split(output, "\n") {
		if key, value, ok := strings.Cut(strings.TrimSpace(line), "="); ok {
			properties[key] = value
		}
	}
	job := Job{State: "idle"}
	if properties["LoadState"] != "loaded" {
		return job, ""
	}
	job.Operation = strings.TrimPrefix(properties["Description"], "mini-ubuntu-server apt ")
	job.StartedAt = parseUnixTimestamp(properties["ExecMainStartTimestamp"])
	switch {
	case properties["ActiveState"] == "failed":
		job.State = "failed"
	case properties["ActiveState"] == "active" && properties["SubState"] == "exited":
		job.State = "succeeded"
	case properties["ActiveState"] == "active" || properties["ActiveState"] == "activating" || properties["ActiveState"] == "deactivating":
		job.State = "running"
	default:
		return Job{State: "idle"}, ""
	}
	if job.State != "running" {
		job.FinishedAt = parseUnixTimestamp(properties["ExecMainExitTimestamp"])
		if code, err := strconv.Atoi(properties["ExecMainStatus"]); err == nil {
			job.ExitCode = &code
		}
	}
	return job, properties["InvocationID"]
}

// parseUnixTimestamp reads "@1700000000" (--timestamp=unix) or "Sun 2026-10-18 07:13:40 UTC".
func parseUnixTimestamp(value string) *time.Time {
	value = strings.TrimSpace(value)
	if strings.HasPrefix(value, "@") {
		seconds, err := strconv.ParseInt(value[1:], 10, 64)
		if err != nil || seconds <= 0 {
			return nil
		}
		at := time.Unix(seconds, 0).UTC()
		return &at
	}
	at, err := time.Parse("Mon 2006-01-02 15:04:05 MST", value)
	if err != nil {
		return nil
	}
	at = at.UTC()
	return &at
}

func trimLog(output string) []string {
	lines := strings.Split(strings.TrimRight(output, "\n"), "\n")
	if len(lines) > maxLogLines {
		lines = lines[len(lines)-maxLogLines:]
	}
	result := make([]string, 0, len(lines))
	for _, line := range lines {
		if len(line) > maxLogLineBytes {
			line = line[:maxLogLineBytes]
		}
		result = append(result, line)
	}
	if len(result) == 1 && result[0] == "" {
		return []string{}
	}
	return result
}
