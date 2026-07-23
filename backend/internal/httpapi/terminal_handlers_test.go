package httpapi

import (
	"context"
	"testing"
	"time"

	"github.com/kantaevsherhan/mini-ubuntu-server-panel/backend/internal/database"
	terminalmanager "github.com/kantaevsherhan/mini-ubuntu-server-panel/backend/internal/terminal"
)

func TestSameOrigin(t *testing.T) {
	valid := []struct{ origin, host string }{
		{"https://panel.example:8443", "panel.example:8443"},
		{"http://LOCALHOST:8080", "localhost:8080"},
	}
	for _, test := range valid {
		if !sameOrigin(test.origin, test.host) {
			t.Fatalf("valid origin rejected: %s %s", test.origin, test.host)
		}
	}
	invalid := []string{"", "null", "file://panel.example", "https://user@panel.example", "https://panel.example/path", "https://other.example"}
	for _, origin := range invalid {
		if sameOrigin(origin, "panel.example") {
			t.Fatalf("invalid origin accepted: %s", origin)
		}
	}
}

func TestTerminalAuthorizationTracksSessionAndRole(t *testing.T) {
	db, _ := testAuthorizedDB(t)
	var user database.User
	var session database.WebSession
	if db.First(&user).Error != nil || db.First(&session).Error != nil {
		t.Fatal("test authorization records missing")
	}
	api := API{DB: db}
	ticket := terminalmanager.Ticket{UserID: user.ID, SessionID: session.ID}
	if !api.terminalAuthorizationValid(context.Background(), ticket) {
		t.Fatal("active administrator session was rejected")
	}
	if err := db.Model(&user).Update("role", "viewer").Error; err != nil {
		t.Fatal(err)
	}
	if api.terminalAuthorizationValid(context.Background(), ticket) {
		t.Fatal("viewer retained terminal authorization")
	}
	if err := db.Model(&user).Updates(map[string]any{"role": "operator", "is_active": true}).Error; err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	if err := db.Model(&session).Update("revoked_at", now).Error; err != nil {
		t.Fatal(err)
	}
	if api.terminalAuthorizationValid(context.Background(), ticket) {
		t.Fatal("revoked web session retained terminal authorization")
	}
}

func TestTerminalTicketProtocol(t *testing.T) {
	ticket := "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMN0123456789"
	header := terminalmanager.WebSocketSubprotocol + ", " + terminalmanager.TicketProtocolPrefix + ticket
	value, ok := terminalTicketProtocol(header)
	if !ok || value != ticket {
		t.Fatalf("valid protocol rejected: %q %t", value, ok)
	}
	for _, invalid := range []string{
		terminalmanager.WebSocketSubprotocol,
		terminalmanager.TicketProtocolPrefix + ticket,
		header + ", " + terminalmanager.TicketProtocolPrefix + ticket,
	} {
		if _, accepted := terminalTicketProtocol(invalid); accepted {
			t.Fatalf("invalid protocol accepted: %s", invalid)
		}
	}
}
