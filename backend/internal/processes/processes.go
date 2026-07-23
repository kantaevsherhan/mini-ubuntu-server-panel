package processes

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const maxSignalRequestBytes = 1024

type Process struct {
	PID         int       `json:"pid"`
	Name        string    `json:"name"`
	Username    string    `json:"username"`
	State       string    `json:"state"`
	CPUPercent  float64   `json:"cpu_percent"`
	MemoryBytes uint64    `json:"memory_bytes"`
	Command     string    `json:"command"`
	StartedAt   time.Time `json:"started_at"`
}

type Controller interface {
	List(context.Context) ([]Process, error)
	Signal(context.Context, int, string) error
}

type Manager struct {
	Executable string
	ProcRoot   string
}

func NewManager() (*Manager, error) {
	executable, err := os.Executable()
	if err != nil {
		return nil, err
	}
	return &Manager{Executable: executable, ProcRoot: "/proc"}, nil
}

func (m Manager) List(ctx context.Context) ([]Process, error) {
	return list(ctx, m.ProcRoot)
}

func (m Manager) Signal(ctx context.Context, pid int, signal string) error {
	request := signalRequest{PID: pid, Signal: signal}
	if _, err := resolveSignal(request); err != nil {
		return err
	}
	payload, err := json.Marshal(request)
	if err != nil {
		return err
	}
	command := exec.CommandContext(ctx, "/usr/bin/sudo", "-n", m.Executable, "privileged-process")
	command.Stdin = strings.NewReader(string(payload))
	if output, err := command.CombinedOutput(); err != nil {
		return fmt.Errorf("privileged process signal failed: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}

type signalRequest struct {
	PID    int    `json:"pid"`
	Signal string `json:"signal"`
}

func RunPrivilegedSignal(input io.Reader) error {
	if os.Geteuid() != 0 {
		return errors.New("privileged-process must run as root")
	}
	decoder := json.NewDecoder(io.LimitReader(input, maxSignalRequestBytes))
	decoder.DisallowUnknownFields()
	var request signalRequest
	if err := decoder.Decode(&request); err != nil {
		return errors.New("invalid process signal request")
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return errors.New("invalid process signal request")
	}
	signal, err := resolveSignal(request)
	if err != nil {
		return err
	}
	return syscall.Kill(request.PID, signal)
}

func resolveSignal(request signalRequest) (syscall.Signal, error) {
	if request.PID <= 1 {
		return 0, errors.New("protected process")
	}
	switch request.Signal {
	case "TERM":
		return syscall.SIGTERM, nil
	case "KILL":
		return syscall.SIGKILL, nil
	case "HUP":
		return syscall.SIGHUP, nil
	default:
		return 0, errors.New("signal is not allowed")
	}
}

func list(ctx context.Context, procRoot string) ([]Process, error) {
	entries, err := os.ReadDir(procRoot)
	if err != nil {
		return nil, err
	}
	uptime, err := readUptime(filepath.Join(procRoot, "uptime"))
	if err != nil {
		return nil, err
	}
	bootTime := time.Now().UTC().Add(-time.Duration(uptime * float64(time.Second)))
	result := make([]Process, 0, len(entries))
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		pid, err := strconv.Atoi(entry.Name())
		if err != nil || !entry.IsDir() {
			continue
		}
		process, err := readProcess(procRoot, pid, uptime, bootTime)
		if err == nil {
			result = append(result, process)
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].CPUPercent > result[j].CPUPercent })
	return result, nil
}

func readProcess(procRoot string, pid int, uptime float64, bootTime time.Time) (Process, error) {
	base := filepath.Join(procRoot, strconv.Itoa(pid))
	statData, err := os.ReadFile(filepath.Join(base, "stat"))
	if err != nil {
		return Process{}, err
	}
	name, fields, err := parseStat(string(statData))
	if err != nil || len(fields) < 22 {
		return Process{}, errors.New("invalid process stat")
	}
	userTicks, _ := strconv.ParseFloat(fields[11], 64)
	systemTicks, _ := strconv.ParseFloat(fields[12], 64)
	startTicks, _ := strconv.ParseFloat(fields[19], 64)
	elapsed := uptime - startTicks/100
	cpu := 0.0
	if elapsed > 0 {
		cpu = ((userTicks + systemTicks) / 100) / elapsed * 100
	}
	status, err := readStatus(filepath.Join(base, "status"))
	if err != nil {
		return Process{}, err
	}
	uid := status["Uid"]
	if separator := strings.IndexByte(uid, '\t'); separator >= 0 {
		uid = uid[:separator]
	}
	username := uid
	if account, lookupErr := user.LookupId(strings.TrimSpace(uid)); lookupErr == nil {
		username = account.Username
	}
	memory := uint64(0)
	if value := strings.Fields(status["VmRSS"]); len(value) > 0 {
		kilobytes, _ := strconv.ParseUint(value[0], 10, 64)
		memory = kilobytes * 1024
	}
	commandData, _ := os.ReadFile(filepath.Join(base, "cmdline"))
	command := strings.TrimSpace(strings.ReplaceAll(string(commandData), "\x00", " "))
	if len(command) > 2048 {
		command = command[:2048]
	}
	if command == "" {
		command = "[" + name + "]"
	}
	return Process{PID: pid, Name: name, Username: username, State: fields[0], CPUPercent: cpu, MemoryBytes: memory, Command: command, StartedAt: bootTime.Add(time.Duration(startTicks/100) * time.Second)}, nil
}

func parseStat(value string) (string, []string, error) {
	open := strings.IndexByte(value, '(')
	close := strings.LastIndex(value, ") ")
	if open < 0 || close <= open {
		return "", nil, errors.New("invalid stat")
	}
	return value[open+1 : close], strings.Fields(value[close+2:]), nil
}

func readStatus(path string) (map[string]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()
	values := make(map[string]string)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		key, value, found := strings.Cut(scanner.Text(), ":")
		if found {
			values[key] = strings.TrimSpace(value)
		}
	}
	return values, scanner.Err()
}

func readUptime(path string) (float64, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	fields := strings.Fields(string(data))
	if len(fields) == 0 {
		return 0, errors.New("invalid uptime")
	}
	return strconv.ParseFloat(fields[0], 64)
}
