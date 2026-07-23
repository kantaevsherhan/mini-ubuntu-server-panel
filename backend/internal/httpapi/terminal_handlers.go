package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/url"
	"strings"
	"time"

	"github.com/gofiber/contrib/websocket"
	"github.com/gofiber/fiber/v2"
	"github.com/kantaevsherhan/mini-ubuntu-server-panel/backend/internal/auth"
	"github.com/kantaevsherhan/mini-ubuntu-server-panel/backend/internal/database"
	terminalmanager "github.com/kantaevsherhan/mini-ubuntu-server-panel/backend/internal/terminal"
)

const (
	terminalMaximumMessage = 16 * 1024
	terminalMaximumInput   = 8 * 1024
	terminalSessionLimit   = 4 * time.Hour
)

type terminalMessage struct {
	Type    string `json:"type"`
	Data    string `json:"data,omitempty"`
	Columns uint16 `json:"columns,omitempty"`
	Rows    uint16 `json:"rows,omitempty"`
}

func (a API) terminalTicket(c *fiber.Ctx) error {
	if a.Terminal == nil || a.Tickets == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"error": "terminal_unavailable"})
	}
	claims := c.Locals("claims").(*auth.Claims)
	value, expiresAt, err := a.Tickets.Issue(claims.UserID, claims.Username, claims.Role, claims.ID, c.IP())
	if err != nil {
		return err
	}
	return c.JSON(fiber.Map{
		"ticket":      value,
		"expires_at":  expiresAt.UTC(),
		"subprotocol": terminalmanager.WebSocketSubprotocol,
	})
}

func (a API) terminalUpgrade(c *fiber.Ctx) error {
	if !websocket.IsWebSocketUpgrade(c) {
		return fiber.ErrUpgradeRequired
	}
	if a.Terminal == nil || a.Tickets == nil {
		return fiber.ErrServiceUnavailable
	}
	if !sameOrigin(c.Get(fiber.HeaderOrigin), c.Get(fiber.HeaderHost)) {
		return fiber.ErrForbidden
	}
	ticketValue, ok := terminalTicketProtocol(c.Get(fiber.HeaderSecWebSocketProtocol))
	if !ok {
		return fiber.ErrUnauthorized
	}
	ticket, err := a.Tickets.Consume(ticketValue, c.IP())
	if err != nil {
		return fiber.ErrUnauthorized
	}
	c.Locals("terminal_ticket", ticket)
	return websocket.New(a.terminalSocket, websocket.Config{
		HandshakeTimeout:  5 * time.Second,
		Subprotocols:      []string{terminalmanager.WebSocketSubprotocol},
		ReadBufferSize:    terminalMaximumMessage,
		WriteBufferSize:   32 * 1024,
		EnableCompression: false,
	})(c)
}

func (a API) terminalSocket(connection *websocket.Conn) {
	ticket, ok := connection.Locals("terminal_ticket").(terminalmanager.Ticket)
	if !ok || !a.Tickets.Acquire(ticket.UserID) {
		_ = connection.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.ClosePolicyViolation, "session limit reached"), time.Now().Add(time.Second))
		return
	}
	defer a.Tickets.Release(ticket.UserID)

	ctx, cancel := context.WithTimeout(context.Background(), terminalSessionLimit)
	defer cancel()
	go a.monitorTerminalAuthorization(ctx, connection, ticket)
	session, err := a.Terminal.Start(ctx, terminalmanager.DefaultColumns, terminalmanager.DefaultRows)
	if err != nil {
		_ = connection.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseInternalServerErr, "terminal unavailable"), time.Now().Add(time.Second))
		return
	}
	defer func() { _ = session.Wait() }()
	defer func() { _ = session.Close() }()
	database.Audit(a.DB, ticket.UserID, "terminal.session.start", "terminal_session", "", `{"commands":"not_recorded"}`, connection.RemoteAddr().String())
	defer database.Audit(a.DB, ticket.UserID, "terminal.session.end", "terminal_session", "", `{"commands":"not_recorded"}`, connection.RemoteAddr().String())

	connection.SetReadLimit(terminalMaximumMessage)
	_ = connection.SetReadDeadline(time.Now().Add(terminalSessionLimit))
	done := make(chan struct{})
	go streamTerminalOutput(connection, session, done)

	windowStarted := time.Now()
	messages := 0
	for {
		messageType, payload, readErr := connection.ReadMessage()
		if readErr != nil {
			break
		}
		if messageType != websocket.TextMessage || len(payload) > terminalMaximumMessage {
			break
		}
		now := time.Now()
		if now.Sub(windowStarted) >= 10*time.Second {
			windowStarted, messages = now, 0
		}
		messages++
		if messages > 120 || handleTerminalMessage(session, payload) != nil {
			_ = connection.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.ClosePolicyViolation, "invalid terminal message"), time.Now().Add(time.Second))
			break
		}
		select {
		case <-done:
			return
		default:
		}
	}
}

func (a API) monitorTerminalAuthorization(ctx context.Context, connection *websocket.Conn, ticket terminalmanager.Ticket) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if !a.terminalAuthorizationValid(ctx, ticket) {
				_ = connection.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.ClosePolicyViolation, "authorization revoked"), time.Now().Add(time.Second))
				_ = connection.Close()
				return
			}
		}
	}
}

func (a API) terminalAuthorizationValid(ctx context.Context, ticket terminalmanager.Ticket) bool {
	var user database.User
	var session database.WebSession
	if a.DB.WithContext(ctx).First(&user, ticket.UserID).Error != nil || !user.IsActive || (user.Role != "admin" && user.Role != "operator") {
		return false
	}
	if a.DB.WithContext(ctx).Where("id = ? AND user_id = ?", ticket.SessionID, ticket.UserID).First(&session).Error != nil {
		return false
	}
	return session.RevokedAt == nil && session.ExpiresAt.After(time.Now().UTC())
}

func streamTerminalOutput(connection *websocket.Conn, session terminalmanager.Session, done chan<- struct{}) {
	defer close(done)
	buffer := make([]byte, 32*1024)
	for {
		count, err := session.Read(buffer)
		if count > 0 && connection.WriteMessage(websocket.BinaryMessage, buffer[:count]) != nil {
			return
		}
		if err != nil {
			if !errors.Is(err, io.EOF) {
				_ = connection.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, "terminal closed"), time.Now().Add(time.Second))
			}
			return
		}
	}
}

func handleTerminalMessage(session terminalmanager.Session, payload []byte) error {
	var message terminalMessage
	if json.Unmarshal(payload, &message) != nil {
		return errors.New("invalid message")
	}
	switch message.Type {
	case "input":
		if len(message.Data) == 0 || len(message.Data) > terminalMaximumInput {
			return errors.New("invalid input")
		}
		_, err := io.WriteString(session, message.Data)
		return err
	case "resize":
		if message.Columns < 20 || message.Columns > 300 || message.Rows < 5 || message.Rows > 120 {
			return errors.New("invalid size")
		}
		return session.Resize(message.Columns, message.Rows)
	default:
		return errors.New("invalid message type")
	}
}

func sameOrigin(origin, host string) bool {
	if origin == "" || host == "" || len(origin) > 512 || len(host) > 255 {
		return false
	}
	parsed, err := url.Parse(origin)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.User != nil || parsed.Path != "" || parsed.RawQuery != "" || parsed.Fragment != "" {
		return false
	}
	return strings.EqualFold(parsed.Host, host)
}

func terminalTicketProtocol(header string) (string, bool) {
	if len(header) > 512 {
		return "", false
	}
	foundBase := false
	ticket := ""
	for _, item := range strings.Split(header, ",") {
		protocol := strings.TrimSpace(item)
		if protocol == terminalmanager.WebSocketSubprotocol {
			foundBase = true
		}
		if strings.HasPrefix(protocol, terminalmanager.TicketProtocolPrefix) {
			if ticket != "" {
				return "", false
			}
			ticket = strings.TrimPrefix(protocol, terminalmanager.TicketProtocolPrefix)
		}
	}
	if !foundBase || len(ticket) < 40 || len(ticket) > terminalmanager.MaximumTicketLength {
		return "", false
	}
	return ticket, true
}
