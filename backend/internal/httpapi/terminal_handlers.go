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
	"gorm.io/gorm"
)

const (
	terminalMaximumMessage = 16 * 1024
	terminalMaximumInput   = 8 * 1024
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
	c.Locals("terminal_session", c.Query("session"))
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
	sessionID, _ := connection.Locals("terminal_session").(string)
	if !ok || a.TerminalHub == nil {
		return
	}
	session, err := a.TerminalHub.Get(ticket.UserID, sessionID)
	if err != nil {
		_ = connection.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(4404, "terminal session not found"), time.Now().Add(time.Second))
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go a.monitorTerminalAuthorization(ctx, connection, ticket)

	replay, output, detach := session.Attach()
	defer detach()
	if len(replay) > 0 && connection.WriteMessage(websocket.BinaryMessage, replay) != nil {
		return
	}
	connection.SetReadLimit(terminalMaximumMessage)
	go streamTerminalOutput(connection, session, output)

	windowStarted := time.Now()
	messages := 0
	for {
		messageType, payload, readErr := connection.ReadMessage()
		if readErr != nil {
			return
		}
		if messageType != websocket.TextMessage || len(payload) > terminalMaximumMessage {
			return
		}
		now := time.Now()
		if now.Sub(windowStarted) >= 10*time.Second {
			windowStarted, messages = now, 0
		}
		messages++
		if messages > 400 || handleTerminalMessage(session, payload) != nil {
			_ = connection.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.ClosePolicyViolation, "invalid terminal message"), time.Now().Add(time.Second))
			return
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

// streamTerminalOutput forwards live shell output; when the channel closes the browser is either replaced,
// too slow (it reconnects and replays scrollback) or the shell exited.
func streamTerminalOutput(connection *websocket.Conn, session *terminalmanager.HubSession, output <-chan []byte) {
	for chunk := range output {
		if connection.WriteMessage(websocket.BinaryMessage, chunk) != nil {
			return
		}
	}
	reason, code := "terminal detached", 4000
	select {
	case <-session.Exited():
		reason, code = "terminal closed", websocket.CloseNormalClosure
	default:
		if session.WasReplaced(output) {
			reason, code = "terminal opened elsewhere", 4001
		}
	}
	_ = connection.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(code, reason), time.Now().Add(time.Second))
	_ = connection.Close()
}

func handleTerminalMessage(session interface {
	io.Writer
	Resize(uint16, uint16) error
}, payload []byte) error {

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

func terminalTitle(value string) string {
	value = strings.TrimSpace(value)
	if len(value) > 40 {
		value = value[:40]
	}
	return strings.ToValidUTF8(value, "")
}

func (a API) terminalSessions(c *fiber.Ctx) error {
	if a.TerminalHub == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"error": "terminal_unavailable"})
	}
	claims := c.Locals("claims").(*auth.Claims)
	return c.JSON(a.TerminalHub.List(claims.UserID))
}

func (a API) terminalCreateSession(c *fiber.Ctx) error {
	if a.TerminalHub == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"error": "terminal_unavailable"})
	}
	var request struct {
		Title   string `json:"title"`
		Columns uint16 `json:"columns"`
		Rows    uint16 `json:"rows"`
	}
	_ = c.BodyParser(&request)
	if request.Columns < 20 || request.Columns > 300 || request.Rows < 5 || request.Rows > 120 {
		request.Columns, request.Rows = terminalmanager.DefaultColumns, terminalmanager.DefaultRows
	}
	claims := c.Locals("claims").(*auth.Claims)
	title := terminalTitle(request.Title)
	if title == "" {
		title = "bash"
	}
	info, err := a.TerminalHub.Create(claims.UserID, title, request.Columns, request.Rows)
	if errors.Is(err, terminalmanager.ErrSessionLimit) {
		return c.Status(fiber.StatusTooManyRequests).JSON(fiber.Map{"error": "terminal_session_limit"})
	}
	if err != nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"error": "terminal_unavailable"})
	}
	database.Audit(a.DB, claims.UserID, "terminal.session.start", "terminal_session", info.ID, `{"commands":"not_recorded"}`, c.IP())
	return c.Status(fiber.StatusCreated).JSON(info)
}

func (a API) terminalRenameSession(c *fiber.Ctx) error {
	if a.TerminalHub == nil {
		return fiber.ErrServiceUnavailable
	}
	var request struct {
		Title string `json:"title"`
	}
	if err := c.BodyParser(&request); err != nil || terminalTitle(request.Title) == "" {
		return fiber.ErrBadRequest
	}
	claims := c.Locals("claims").(*auth.Claims)
	if err := a.TerminalHub.Rename(claims.UserID, c.Params("id"), terminalTitle(request.Title)); err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "terminal_session_not_found"})
	}
	return c.SendStatus(fiber.StatusNoContent)
}

func (a API) terminalCloseSession(c *fiber.Ctx) error {
	if a.TerminalHub == nil {
		return fiber.ErrServiceUnavailable
	}
	claims := c.Locals("claims").(*auth.Claims)
	if err := a.TerminalHub.Close(claims.UserID, c.Params("id")); errors.Is(err, terminalmanager.ErrSessionNotFound) {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "terminal_session_not_found"})
	}
	return c.SendStatus(fiber.StatusNoContent)
}

func (a API) terminalUserAllowed(ctx context.Context, userID int64) bool {
	var user database.User
	if err := a.DB.WithContext(ctx).First(&user, userID).Error; err != nil {
		return !errors.Is(err, gorm.ErrRecordNotFound)
	}
	return user.IsActive && (user.Role == "admin" || user.Role == "operator")
}
