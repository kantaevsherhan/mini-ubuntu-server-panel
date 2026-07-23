package terminal

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"sync"
	"time"
)

const (
	TicketLifetime       = 30 * time.Second
	MaxSessionsPerUser   = 2
	WebSocketSubprotocol = "mini-ubuntu-terminal"
	TicketProtocolPrefix = "ticket."
	MaximumTicketLength  = 128
)

type Ticket struct {
	UserID    int64
	Username  string
	Role      string
	SessionID string
	IP        string
	ExpiresAt time.Time
}

type TicketStore struct {
	mu      sync.Mutex
	tickets map[[32]byte]Ticket
	active  map[int64]int
	now     func() time.Time
}

func NewTicketStore() *TicketStore {
	return &TicketStore{
		tickets: make(map[[32]byte]Ticket),
		active:  make(map[int64]int),
		now:     time.Now,
	}
}

func (s *TicketStore) Issue(userID int64, username, role, sessionID, ip string) (string, time.Time, error) {
	buffer := make([]byte, 32)
	if _, err := rand.Read(buffer); err != nil {
		return "", time.Time{}, errors.New("failed to issue terminal ticket")
	}
	value := base64.RawURLEncoding.EncodeToString(buffer)
	expires := s.now().Add(TicketLifetime)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pruneLocked()
	s.tickets[sha256.Sum256([]byte(value))] = Ticket{
		UserID: userID, Username: username, Role: role, SessionID: sessionID, IP: ip, ExpiresAt: expires,
	}
	return value, expires, nil
}

func (s *TicketStore) Consume(value, ip string) (Ticket, error) {
	if len(value) < 40 || len(value) > MaximumTicketLength {
		return Ticket{}, errors.New("invalid terminal ticket")
	}
	key := sha256.Sum256([]byte(value))
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pruneLocked()
	ticket, exists := s.tickets[key]
	delete(s.tickets, key)
	if !exists || ticket.IP != ip {
		return Ticket{}, errors.New("invalid terminal ticket")
	}
	return ticket, nil
}

func (s *TicketStore) Acquire(userID int64) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.active[userID] >= MaxSessionsPerUser {
		return false
	}
	s.active[userID]++
	return true
}

func (s *TicketStore) Release(userID int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.active[userID] <= 1 {
		delete(s.active, userID)
		return
	}
	s.active[userID]--
}

func (s *TicketStore) pruneLocked() {
	now := s.now()
	for key, ticket := range s.tickets {
		if !ticket.ExpiresAt.After(now) {
			delete(s.tickets, key)
		}
	}
}
