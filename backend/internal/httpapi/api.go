package httpapi

import (
	"context"
	"database/sql"
	"fmt"
	"net"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/limiter"
	"github.com/kantaevsherhan/mini-ubuntu-server-panel/backend/internal/auth"
	"github.com/kantaevsherhan/mini-ubuntu-server-panel/backend/internal/database"
	"github.com/kantaevsherhan/mini-ubuntu-server-panel/backend/internal/systemusers"
)

var usernamePattern = regexp.MustCompile(`^[a-z_][a-z0-9_-]{2,31}$`)

type API struct {
	DB      *sql.DB
	Secret  string
	Version string
}

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type createUserRequest struct {
	Username       string `json:"username"`
	DisplayName    string `json:"display_name"`
	Password       string `json:"password"`
	Role           string `json:"role"`
	SystemUsername string `json:"system_username"`
}

func (a API) Register(app *fiber.App) {
	api := app.Group("/api/v1")
	api.Get("/health", a.health)
	api.Post("/auth/login", limiter.New(limiter.Config{
		Max:        5,
		Expiration: 15 * time.Minute,
		LimitReached: func(c *fiber.Ctx) error {
			database.Audit(a.DB, nil, "auth.rate_limited", "ip", "", "{}", c.IP())
			return c.Status(fiber.StatusTooManyRequests).JSON(fiber.Map{"error": "too_many_login_attempts"})
		},
	}), a.login)

	secured := api.Group("", a.authorize)
	secured.Get("/me", func(c *fiber.Ctx) error { return c.JSON(c.Locals("claims")) })
	secured.Get("/dashboard", a.dashboard)
	secured.Get("/users", a.users)
	secured.Post("/users", a.requireRole("admin"), a.createUser)
	secured.Get("/system-users", a.requireRole("admin", "operator"), a.systemUsers)
	secured.Get("/telegram/settings", a.requireRole("admin"), a.telegramSettings)
	secured.Put("/telegram/settings", a.requireRole("admin"), a.updateTelegramSettings)
	secured.Get("/audit", a.requireRole("admin"), a.audit)
}

func (a API) health(c *fiber.Ctx) error {
	ctx, cancel := context.WithTimeout(c.UserContext(), 2*time.Second)
	defer cancel()
	if err := a.DB.PingContext(ctx); err != nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"status": "error"})
	}
	return c.JSON(fiber.Map{"status": "ok", "version": a.Version})
}

func (a API) login(c *fiber.Ctx) error {
	var request loginRequest
	if err := c.BodyParser(&request); err != nil {
		return fiber.ErrBadRequest
	}
	request.Username = strings.TrimSpace(request.Username)
	if !usernamePattern.MatchString(request.Username) || len(request.Password) > 1024 {
		return invalidCredentials(c, a.DB, request.Username)
	}

	var id int64
	var hash, role string
	var active, mustChange bool
	err := a.DB.QueryRowContext(c.UserContext(), `SELECT id,password_hash,role,is_active,must_change_password FROM users WHERE username=?`, request.Username).Scan(&id, &hash, &role, &active, &mustChange)
	if err != nil || !active || !auth.Verify(hash, request.Password) {
		return invalidCredentials(c, a.DB, request.Username)
	}

	token, err := auth.Sign(a.Secret, id, request.Username, role)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	_, _ = a.DB.ExecContext(c.UserContext(), `UPDATE users SET last_login_at=?,updated_at=? WHERE id=?`, now, now, id)
	database.Audit(a.DB, id, "auth.login", "user", strconv.FormatInt(id, 10), "{}", c.IP())
	return c.JSON(fiber.Map{"access_token": token, "token_type": "Bearer", "must_change_password": mustChange})
}

func invalidCredentials(c *fiber.Ctx, db *sql.DB, username string) error {
	database.Audit(db, nil, "auth.login_failed", "user", username, "{}", c.IP())
	return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "invalid_credentials"})
}

func (a API) authorize(c *fiber.Ctx) error {
	header := c.Get(fiber.HeaderAuthorization)
	if !strings.HasPrefix(header, "Bearer ") {
		return fiber.ErrUnauthorized
	}
	claims, err := auth.Parse(a.Secret, strings.TrimPrefix(header, "Bearer "))
	if err != nil {
		return fiber.ErrUnauthorized
	}
	var role string
	var active bool
	if err := a.DB.QueryRowContext(c.UserContext(), `SELECT role,is_active FROM users WHERE id=?`, claims.UserID).Scan(&role, &active); err != nil || !active {
		return fiber.ErrUnauthorized
	}
	claims.Role = role
	c.Locals("claims", claims)
	return c.Next()
}

func (a API) requireRole(roles ...string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		claims := c.Locals("claims").(*auth.Claims)
		for _, role := range roles {
			if claims.Role == role {
				return c.Next()
			}
		}
		return fiber.ErrForbidden
	}
}

func (a API) dashboard(c *fiber.Ctx) error {
	var users, events int
	if err := a.DB.QueryRowContext(c.UserContext(), `SELECT count(*) FROM users WHERE is_active=1`).Scan(&users); err != nil {
		return err
	}
	if err := a.DB.QueryRowContext(c.UserContext(), `SELECT count(*) FROM notification_events WHERE status='pending'`).Scan(&events); err != nil {
		return err
	}
	hostname, _ := os.Hostname()
	return c.JSON(fiber.Map{"hostname": hostname, "panel_users": users, "pending_notifications": events, "status": "online"})
}

func (a API) users(c *fiber.Ctx) error {
	rows, err := a.DB.QueryContext(c.UserContext(), `SELECT id,username,display_name,role,is_active,system_username,created_at,last_login_at FROM users ORDER BY username`)
	if err != nil {
		return err
	}
	defer rows.Close()
	result := make([]fiber.Map, 0)
	for rows.Next() {
		var id int
		var username, displayName, role string
		var active bool
		var systemUsername sql.NullString
		var createdAt time.Time
		var lastLoginAt sql.NullTime
		if err := rows.Scan(&id, &username, &displayName, &role, &active, &systemUsername, &createdAt, &lastLoginAt); err != nil {
			return err
		}
		result = append(result, fiber.Map{"id": id, "username": username, "display_name": displayName, "role": role, "is_active": active, "system_username": nullableString(systemUsername), "created_at": createdAt, "last_login_at": nullableTime(lastLoginAt)})
	}
	return c.JSON(result)
}

func (a API) createUser(c *fiber.Ctx) error {
	var request createUserRequest
	if err := c.BodyParser(&request); err != nil {
		return fiber.ErrBadRequest
	}
	request.Username = strings.TrimSpace(request.Username)
	request.SystemUsername = strings.TrimSpace(request.SystemUsername)
	request.DisplayName = strings.TrimSpace(request.DisplayName)
	if !usernamePattern.MatchString(request.Username) || len(request.Password) < 12 || len(request.Password) > 1024 || len(request.DisplayName) > 128 {
		return c.Status(fiber.StatusUnprocessableEntity).JSON(fiber.Map{"error": "user_validation_failed"})
	}
	if request.SystemUsername != "" && !usernamePattern.MatchString(request.SystemUsername) {
		return c.Status(fiber.StatusUnprocessableEntity).JSON(fiber.Map{"error": "system_username_invalid"})
	}
	if request.Role != "admin" && request.Role != "operator" && request.Role != "viewer" {
		return c.Status(fiber.StatusUnprocessableEntity).JSON(fiber.Map{"error": "role_invalid"})
	}
	hash, err := auth.Hash(request.Password)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	result, err := a.DB.ExecContext(c.UserContext(), `INSERT INTO users(username,display_name,password_hash,role,system_username,created_at,updated_at) VALUES(?,?,?,?,NULLIF(?,''),?,?)`, request.Username, request.DisplayName, hash, request.Role, request.SystemUsername, now, now)
	if err != nil {
		return c.Status(fiber.StatusConflict).JSON(fiber.Map{"error": "username_exists"})
	}
	id, err := result.LastInsertId()
	if err != nil {
		return err
	}
	claims := c.Locals("claims").(*auth.Claims)
	database.Audit(a.DB, claims.UserID, "user.create", "user", fmt.Sprint(id), `{"password":"hidden"}`, c.IP())
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"id": id})
}

func (a API) systemUsers(c *fiber.Ctx) error {
	users, err := systemusers.List()
	if err != nil {
		return err
	}
	return c.JSON(users)
}

func (a API) telegramSettings(c *fiber.Ctx) error {
	var enabled bool
	var apiBaseURL string
	var timeout, retryCount, recipientCount int
	err := a.DB.QueryRowContext(c.UserContext(), `SELECT enabled,api_base_url,request_timeout_seconds,retry_count FROM telegram_settings WHERE id=1`).Scan(&enabled, &apiBaseURL, &timeout, &retryCount)
	if err != nil {
		return err
	}
	if err := a.DB.QueryRowContext(c.UserContext(), `SELECT count(*) FROM telegram_recipients WHERE enabled=1`).Scan(&recipientCount); err != nil {
		return err
	}
	return c.JSON(fiber.Map{"enabled": enabled, "api_base_url": apiBaseURL, "request_timeout_seconds": timeout, "retry_count": retryCount, "recipient_count": recipientCount, "token_configured": os.Getenv("MINI_UBUNTU_SERVER_TELEGRAM_BOT_TOKEN") != ""})
}

func (a API) updateTelegramSettings(c *fiber.Ctx) error {
	var request struct {
		Enabled        bool   `json:"enabled"`
		APIBaseURL     string `json:"api_base_url"`
		RequestTimeout int    `json:"request_timeout_seconds"`
		RetryCount     int    `json:"retry_count"`
	}
	if err := c.BodyParser(&request); err != nil {
		return fiber.ErrBadRequest
	}
	request.APIBaseURL = strings.TrimRight(strings.TrimSpace(request.APIBaseURL), "/")
	if err := validateTelegramAPIURL(c.UserContext(), request.APIBaseURL); err != nil || request.RequestTimeout < 1 || request.RequestTimeout > 60 || request.RetryCount < 0 || request.RetryCount > 10 {
		return c.Status(fiber.StatusUnprocessableEntity).JSON(fiber.Map{"error": "telegram_settings_invalid"})
	}
	_, err := a.DB.ExecContext(c.UserContext(), `UPDATE telegram_settings SET enabled=?,api_base_url=?,request_timeout_seconds=?,retry_count=?,updated_at=? WHERE id=1`, request.Enabled, request.APIBaseURL, request.RequestTimeout, request.RetryCount, time.Now().UTC())
	if err != nil {
		return err
	}
	claims := c.Locals("claims").(*auth.Claims)
	database.Audit(a.DB, claims.UserID, "telegram.settings.update", "telegram", "1", `{"token_value":"hidden"}`, c.IP())
	return c.SendStatus(fiber.StatusNoContent)
}

func validateTelegramAPIURL(ctx context.Context, rawURL string) error {
	parsed, err := url.ParseRequestURI(rawURL)
	if err != nil || parsed.Hostname() == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return fmt.Errorf("invalid API URL")
	}
	if parsed.Scheme == "http" {
		ip := net.ParseIP(parsed.Hostname())
		if ip == nil || !ip.IsLoopback() {
			return fmt.Errorf("plain HTTP is restricted to loopback")
		}
		return nil
	}
	if parsed.Scheme != "https" {
		return fmt.Errorf("HTTPS is required")
	}
	lookupCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	addresses, err := net.DefaultResolver.LookupIPAddr(lookupCtx, parsed.Hostname())
	if err != nil || len(addresses) == 0 {
		return fmt.Errorf("unable to resolve API host")
	}
	for _, address := range addresses {
		if address.IP.IsPrivate() || address.IP.IsLoopback() || address.IP.IsLinkLocalUnicast() || address.IP.IsUnspecified() {
			return fmt.Errorf("private API destination is not allowed over HTTPS")
		}
	}
	return nil
}

func (a API) audit(c *fiber.Ctx) error {
	rows, err := a.DB.QueryContext(c.UserContext(), `SELECT id,action,target_type,COALESCE(target_id,''),details_json,created_at FROM audit_events ORDER BY id DESC LIMIT 200`)
	if err != nil {
		return err
	}
	defer rows.Close()
	result := make([]fiber.Map, 0)
	for rows.Next() {
		var id int
		var action, target, targetID, details string
		var createdAt time.Time
		if err := rows.Scan(&id, &action, &target, &targetID, &details, &createdAt); err != nil {
			return err
		}
		result = append(result, fiber.Map{"id": id, "action": action, "target_type": target, "target_id": targetID, "details": details, "created_at": createdAt})
	}
	return c.JSON(result)
}

func nullableString(value sql.NullString) any {
	if !value.Valid {
		return nil
	}
	return value.String
}

func nullableTime(value sql.NullTime) any {
	if !value.Valid {
		return nil
	}
	return value.Time
}
