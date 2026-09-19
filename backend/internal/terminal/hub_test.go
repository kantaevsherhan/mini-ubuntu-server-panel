package terminal

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"
)

// echoSession echoes input back as output, like a shell with local echo.
type echoSession struct {
	reader *io.PipeReader
	writer *io.PipeWriter
}

func newEchoSession() *echoSession {
	reader, writer := io.Pipe()
	return &echoSession{reader: reader, writer: writer}
}

func (s *echoSession) Read(buffer []byte) (int, error)  { return s.reader.Read(buffer) }
func (s *echoSession) Write(buffer []byte) (int, error) { return s.writer.Write(buffer) }
func (s *echoSession) Resize(uint16, uint16) error      { return nil }
func (s *echoSession) Close() error                     { return s.writer.Close() }
func (s *echoSession) Wait() error                      { return nil }

type echoController struct{}

func (echoController) Start(context.Context, uint16, uint16) (Session, error) {
	return newEchoSession(), nil
}

func receive(t *testing.T, output <-chan []byte, want string) {
	t.Helper()
	var got strings.Builder
	deadline := time.After(2 * time.Second)
	for !strings.Contains(got.String(), want) {
		select {
		case chunk, ok := <-output:
			if !ok {
				t.Fatalf("output closed, got %q", got.String())
			}
			got.Write(chunk)
		case <-deadline:
			t.Fatalf("timeout waiting for %q, got %q", want, got.String())
		}
	}
}

func TestHubSessionSurvivesDetachAndReplaysScrollback(t *testing.T) {
	hub := NewHub(echoController{})
	exited := make(chan string, 1)
	hub.OnExit = func(_ int64, id string) { exited <- id }
	info, err := hub.Create(7, "work", 80, 24)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := hub.Get(8, info.ID); err == nil {
		t.Fatal("another user can access the session")
	}
	session, _ := hub.Get(7, info.ID)
	_, output, detach := session.Attach()
	_, _ = session.Write([]byte("hello\n"))
	receive(t, output, "hello")
	detach()

	_, _ = session.Write([]byte("while-away\n"))
	time.Sleep(50 * time.Millisecond)
	replay, output, detach := session.Attach()
	defer detach()
	if !strings.Contains(string(replay), "hello") || !strings.Contains(string(replay), "while-away") {
		t.Fatalf("scrollback not replayed: %q", replay)
	}
	if list := hub.List(7); len(list) != 1 || !list[0].Attached || list[0].Title != "work" {
		t.Fatalf("unexpected list: %#v", list)
	}

	if err := hub.Close(7, info.ID); err != nil {
		t.Fatal(err)
	}
	if id := <-exited; id != info.ID || len(hub.List(7)) != 0 {
		t.Fatalf("session not removed: %q", id)
	}
	if _, ok := <-output; ok {
		t.Fatal("attachment still open after exit")
	}
}

func TestHubLimitsSessionsPerUser(t *testing.T) {
	hub := NewHub(echoController{})
	for range MaxHubSessionsPerUser {
		if _, err := hub.Create(1, "x", 80, 24); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := hub.Create(1, "x", 80, 24); err != ErrSessionLimit {
		t.Fatalf("limit not enforced: %v", err)
	}
	hub.CloseUser(1)
	if len(hub.List(1)) != 0 {
		t.Fatal("CloseUser left sessions")
	}
}
