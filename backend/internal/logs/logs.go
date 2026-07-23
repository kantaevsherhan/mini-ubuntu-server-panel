package logs

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
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	maxRequestBytes = 4096
	maxMessageBytes = 8192
)

var unitPattern = regexp.MustCompile(`^[A-Za-z0-9_.@:-]{1,240}\.service$`)

type Query struct {
	Unit     string `json:"unit"`
	Priority string `json:"priority"`
	Range    string `json:"range"`
	Limit    int    `json:"limit"`
}

type Entry struct {
	Timestamp  time.Time `json:"timestamp"`
	Unit       string    `json:"unit"`
	Priority   string    `json:"priority"`
	Message    string    `json:"message"`
	Identifier string    `json:"identifier"`
	PID        string    `json:"pid"`
}

type Controller interface {
	List(context.Context, Query) ([]Entry, error)
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

func (m Manager) List(ctx context.Context, query Query) ([]Entry, error) {
	query = Normalize(query)
	if err := Validate(query); err != nil {
		return nil, err
	}
	payload, err := json.Marshal(query)
	if err != nil {
		return nil, err
	}
	command := exec.CommandContext(ctx, "/usr/bin/sudo", "-n", m.Executable, "privileged-logs")
	command.Stdin = bytes.NewReader(payload)
	output, err := command.Output()
	if err != nil {
		return nil, fmt.Errorf("privileged logs query failed: %w", err)
	}
	var entries []Entry
	if err := json.Unmarshal(output, &entries); err != nil {
		return nil, errors.New("invalid logs helper response")
	}
	return entries, nil
}

func RunPrivileged(input io.Reader, output io.Writer) error {
	if os.Geteuid() != 0 {
		return errors.New("privileged-logs must run as root")
	}
	decoder := json.NewDecoder(io.LimitReader(input, maxRequestBytes))
	decoder.DisallowUnknownFields()
	var query Query
	if err := decoder.Decode(&query); err != nil {
		return errors.New("invalid logs request")
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return errors.New("invalid logs request")
	}
	query = Normalize(query)
	if err := Validate(query); err != nil {
		return err
	}
	args := []string{"--no-pager", "--output=json", "--reverse", "--lines=" + strconv.Itoa(query.Limit), "--since=" + sinceValue(query.Range), "--priority=" + query.Priority}
	if query.Unit != "" {
		args = append(args, "--unit="+query.Unit)
	}
	command := exec.Command("/usr/bin/journalctl", args...)
	command.Env = append(os.Environ(), "LC_ALL=C")
	data, err := command.Output()
	if err != nil {
		return fmt.Errorf("journal query failed: %w", err)
	}
	return json.NewEncoder(output).Encode(parseEntries(data, query.Limit))
}

func Normalize(query Query) Query {
	query.Unit = strings.TrimSpace(query.Unit)
	query.Priority = strings.ToLower(strings.TrimSpace(query.Priority))
	query.Range = strings.ToLower(strings.TrimSpace(query.Range))
	if query.Priority == "" {
		query.Priority = "info"
	}
	if query.Range == "" {
		query.Range = "day"
	}
	if query.Limit == 0 {
		query.Limit = 1000
	}
	return query
}

func Validate(query Query) error {
	if query.Unit != "" && !unitPattern.MatchString(query.Unit) {
		return errors.New("invalid log unit")
	}
	switch query.Priority {
	case "emerg", "alert", "crit", "err", "warning", "notice", "info", "debug":
	default:
		return errors.New("invalid log priority")
	}
	switch query.Range {
	case "hour", "day", "week":
	default:
		return errors.New("invalid log range")
	}
	if query.Limit < 1 || query.Limit > 2000 {
		return errors.New("invalid log limit")
	}
	return nil
}

func sinceValue(value string) string {
	switch value {
	case "hour":
		return "1 hour ago"
	case "week":
		return "7 days ago"
	default:
		return "24 hours ago"
	}
}

type journalEntry struct {
	Timestamp  string `json:"__REALTIME_TIMESTAMP"`
	Unit       string `json:"_SYSTEMD_UNIT"`
	Priority   string `json:"PRIORITY"`
	Message    string `json:"MESSAGE"`
	Identifier string `json:"SYSLOG_IDENTIFIER"`
	Command    string `json:"_COMM"`
	PID        string `json:"_PID"`
}

func parseEntries(data []byte, limit int) []Entry {
	entries := make([]Entry, 0, min(limit, 256))
	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		var raw journalEntry
		if json.Unmarshal(scanner.Bytes(), &raw) != nil {
			continue
		}
		micros, err := strconv.ParseInt(raw.Timestamp, 10, 64)
		if err != nil {
			continue
		}
		identifier := raw.Identifier
		if identifier == "" {
			identifier = raw.Command
		}
		message := raw.Message
		if len(message) > maxMessageBytes {
			message = message[:maxMessageBytes]
		}
		entries = append(entries, Entry{Timestamp: time.UnixMicro(micros).UTC(), Unit: raw.Unit, Priority: raw.Priority, Message: message, Identifier: identifier, PID: raw.PID})
		if len(entries) >= limit {
			break
		}
	}
	return entries
}
