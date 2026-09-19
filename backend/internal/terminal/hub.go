package terminal

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"sort"
	"sync"
	"time"
)

const (
	// MaxHubSessionsPerUser bounds how many persistent shells one panel user may keep open.
	MaxHubSessionsPerUser = 8
	// scrollbackBytes is replayed to a browser when it re-attaches to a running shell.
	scrollbackBytes = 256 * 1024
	attachBuffer    = 256
)

var (
	ErrSessionNotFound = errors.New("terminal session not found")
	ErrSessionLimit    = errors.New("terminal session limit reached")
)

// SessionInfo is the browser-visible description of a persistent shell.
type SessionInfo struct {
	ID           string    `json:"id"`
	Title        string    `json:"title"`
	CreatedAt    time.Time `json:"created_at"`
	LastActiveAt time.Time `json:"last_active_at"`
	Attached     bool      `json:"attached"`
}

// HubSession is a shell that keeps running while no browser is connected.
type HubSession struct {
	id        string
	userID    int64
	title     string
	createdAt time.Time
	session   Session

	mu         sync.Mutex
	lastActive time.Time
	scrollback []byte
	attached   chan []byte
	replaced   chan []byte
	exited     chan struct{}
}

func (s *HubSession) ID() string    { return s.id }
func (s *HubSession) UserID() int64 { return s.userID }

// Exited is closed when the shell process ends.
func (s *HubSession) Exited() <-chan struct{} { return s.exited }

func (s *HubSession) info() SessionInfo {
	s.mu.Lock()
	defer s.mu.Unlock()
	return SessionInfo{ID: s.id, Title: s.title, CreatedAt: s.createdAt, LastActiveAt: s.lastActive, Attached: s.attached != nil}
}

func (s *HubSession) Write(data []byte) (int, error) {
	s.touch()
	return s.session.Write(data)
}

func (s *HubSession) Resize(columns, rows uint16) error { return s.session.Resize(columns, rows) }

func (s *HubSession) touch() {
	s.mu.Lock()
	s.lastActive = time.Now().UTC()
	s.mu.Unlock()
}

// Attach replaces any previous browser attachment and returns the scrollback plus a live output channel.
// The channel is closed when the attachment is replaced, falls too far behind, or the shell exits.
func (s *HubSession) Attach() ([]byte, <-chan []byte, func()) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.attached != nil {
		close(s.attached)
		s.replaced = s.attached
	}
	channel := make(chan []byte, attachBuffer)
	s.attached = channel
	replay := append([]byte(nil), s.scrollback...)
	select {
	case <-s.exited:
		close(channel)
		s.attached = nil
	default:
	}
	detach := func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		if s.attached == channel {
			close(channel)
			s.attached = nil
		}
	}
	return replay, channel, detach
}

// WasReplaced reports whether the attachment ended because another browser took over the session.
func (s *HubSession) WasReplaced(output <-chan []byte) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.replaced != nil && (<-chan []byte)(s.replaced) == output
}

func (s *HubSession) pump(onExit func()) {
	buffer := make([]byte, 32*1024)
	for {
		count, err := s.session.Read(buffer)
		if count > 0 {
			chunk := append([]byte(nil), buffer[:count]...)
			s.mu.Lock()
			s.scrollback = append(s.scrollback, chunk...)
			if overflow := len(s.scrollback) - scrollbackBytes; overflow > 0 {
				s.scrollback = append([]byte(nil), s.scrollback[overflow:]...)
			}
			if s.attached != nil {
				select {
				case s.attached <- chunk:
				default:
					// Slow browser: drop it; it will re-attach and replay the scrollback.
					close(s.attached)
					s.attached = nil
				}
			}
			s.mu.Unlock()
		}
		if err != nil {
			break
		}
	}
	_ = s.session.Close()
	_ = s.session.Wait()
	s.mu.Lock()
	close(s.exited)
	if s.attached != nil {
		close(s.attached)
		s.attached = nil
	}
	s.mu.Unlock()
	onExit()
}

// Hub owns persistent shells so that closing the browser tab does not end them.
type Hub struct {
	controller Controller
	mu         sync.Mutex
	sessions   map[string]*HubSession
	// OnExit is called after a shell ends (process exit or explicit close).
	OnExit func(userID int64, id string)
}

func NewHub(controller Controller) *Hub {
	return &Hub{controller: controller, sessions: map[string]*HubSession{}}
}

func (h *Hub) Create(userID int64, title string, columns, rows uint16) (SessionInfo, error) {
	h.mu.Lock()
	count := 0
	for _, session := range h.sessions {
		if session.userID == userID {
			count++
		}
	}
	h.mu.Unlock()
	if count >= MaxHubSessionsPerUser {
		return SessionInfo{}, ErrSessionLimit
	}
	session, err := h.controller.Start(context.Background(), columns, rows)
	if err != nil {
		return SessionInfo{}, err
	}
	raw := make([]byte, 12)
	if _, err := rand.Read(raw); err != nil {
		_ = session.Close()
		return SessionInfo{}, err
	}
	now := time.Now().UTC()
	item := &HubSession{id: hex.EncodeToString(raw), userID: userID, title: title, createdAt: now, lastActive: now, session: session, exited: make(chan struct{})}
	h.mu.Lock()
	h.sessions[item.id] = item
	h.mu.Unlock()
	go item.pump(func() {
		h.mu.Lock()
		delete(h.sessions, item.id)
		h.mu.Unlock()
		if h.OnExit != nil {
			h.OnExit(userID, item.id)
		}
	})
	return item.info(), nil
}

func (h *Hub) List(userID int64) []SessionInfo {
	h.mu.Lock()
	items := make([]*HubSession, 0)
	for _, session := range h.sessions {
		if session.userID == userID {
			items = append(items, session)
		}
	}
	h.mu.Unlock()
	result := make([]SessionInfo, 0, len(items))
	for _, item := range items {
		result = append(result, item.info())
	}
	sort.Slice(result, func(i, j int) bool { return result[i].CreatedAt.Before(result[j].CreatedAt) })
	return result
}

func (h *Hub) Get(userID int64, id string) (*HubSession, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	session, ok := h.sessions[id]
	if !ok || session.userID != userID {
		return nil, ErrSessionNotFound
	}
	return session, nil
}

func (h *Hub) Rename(userID int64, id, title string) error {
	session, err := h.Get(userID, id)
	if err != nil {
		return err
	}
	session.mu.Lock()
	session.title = title
	session.mu.Unlock()
	return nil
}

// Close kills the shell; the pump goroutine removes it from the hub.
func (h *Hub) Close(userID int64, id string) error {
	session, err := h.Get(userID, id)
	if err != nil {
		return err
	}
	err = session.session.Close()
	<-session.exited
	return err
}

// Users returns every user that currently owns a shell.
func (h *Hub) Users() []int64 {
	h.mu.Lock()
	defer h.mu.Unlock()
	seen := map[int64]bool{}
	users := []int64{}
	for _, session := range h.sessions {
		if !seen[session.userID] {
			seen[session.userID] = true
			users = append(users, session.userID)
		}
	}
	return users
}

func (h *Hub) CloseUser(userID int64) {
	for _, info := range h.List(userID) {
		_ = h.Close(userID, info.ID)
	}
}

// Reap periodically closes shells whose owner is no longer allowed to use the terminal.
func (h *Hub) Reap(ctx context.Context, interval time.Duration, allowed func(context.Context, int64) bool) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			for _, userID := range h.Users() {
				if !allowed(ctx, userID) {
					h.CloseUser(userID)
				}
			}
		}
	}
}
