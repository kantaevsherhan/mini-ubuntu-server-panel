package terminal

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"sync"

	"github.com/creack/pty"
)

const (
	DefaultColumns = 120
	DefaultRows    = 32
)

// Session is the deliberately small PTY surface used by the WebSocket layer.
// The shell always runs with the panel service account and never through sudo.
type Session interface {
	io.Reader
	io.Writer
	Resize(columns, rows uint16) error
	Close() error
	Wait() error
}

type Controller interface {
	Start(ctx context.Context, columns, rows uint16) (Session, error)
}

type Manager struct {
	shell string
}

func NewManager() (*Manager, error) {
	const shell = "/bin/bash"
	info, err := os.Stat(shell)
	if err != nil || !info.Mode().IsRegular() {
		return nil, errors.New("terminal shell is unavailable")
	}
	return &Manager{shell: shell}, nil
}

func (m *Manager) Start(ctx context.Context, columns, rows uint16) (Session, error) {
	if columns < 20 || columns > 300 || rows < 5 || rows > 120 {
		return nil, errors.New("invalid terminal size")
	}
	command := exec.CommandContext(ctx, m.shell, "--noprofile", "--norc", "-i")
	command.Env = append(os.Environ(), "TERM=xterm-256color")
	file, err := pty.StartWithSize(command, &pty.Winsize{Cols: columns, Rows: rows})
	if err != nil {
		return nil, errors.New("failed to start terminal")
	}
	return &ptySession{file: file, command: command}, nil
}

type ptySession struct {
	file    *os.File
	command *exec.Cmd
	once    sync.Once
}

func (s *ptySession) Read(buffer []byte) (int, error)  { return s.file.Read(buffer) }
func (s *ptySession) Write(buffer []byte) (int, error) { return s.file.Write(buffer) }

func (s *ptySession) Resize(columns, rows uint16) error {
	return pty.Setsize(s.file, &pty.Winsize{Cols: columns, Rows: rows})
}

func (s *ptySession) Close() error {
	var closeErr error
	s.once.Do(func() {
		closeErr = s.file.Close()
		if s.command.Process != nil {
			_ = s.command.Process.Kill()
		}
	})
	return closeErr
}

func (s *ptySession) Wait() error { return s.command.Wait() }
