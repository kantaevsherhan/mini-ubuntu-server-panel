package terminal

import (
	"testing"
	"time"
)

func TestTicketIsBoundToIPAndSingleUse(t *testing.T) {
	store := NewTicketStore()
	value, _, err := store.Issue(7, "operator", "operator", "session-1", "192.0.2.1")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Consume(value, "192.0.2.2"); err == nil {
		t.Fatal("ticket accepted for a different IP")
	}
	if _, err := store.Consume(value, "192.0.2.1"); err == nil {
		t.Fatal("failed consume must still invalidate ticket")
	}

	value, _, err = store.Issue(7, "operator", "operator", "session-1", "192.0.2.1")
	if err != nil {
		t.Fatal(err)
	}
	ticket, err := store.Consume(value, "192.0.2.1")
	if err != nil || ticket.UserID != 7 {
		t.Fatalf("valid ticket rejected: %#v %v", ticket, err)
	}
	if _, err := store.Consume(value, "192.0.2.1"); err == nil {
		t.Fatal("ticket was reusable")
	}
}

func TestTicketExpiryAndConcurrentSessionLimit(t *testing.T) {
	store := NewTicketStore()
	now := time.Unix(1_000, 0)
	store.now = func() time.Time { return now }
	value, _, err := store.Issue(9, "admin", "admin", "session-2", "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	now = now.Add(TicketLifetime + time.Second)
	if _, err := store.Consume(value, "127.0.0.1"); err == nil {
		t.Fatal("expired ticket accepted")
	}
	first := store.Acquire(9)
	second := store.Acquire(9)
	third := store.Acquire(9)
	if !first || !second || third {
		t.Fatal("per-user session limit not enforced")
	}
	store.Release(9)
	if !store.Acquire(9) {
		t.Fatal("released capacity was not available")
	}
}
