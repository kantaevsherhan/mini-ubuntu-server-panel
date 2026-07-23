package httpapi

import (
	"context"
	"errors"
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
	secretstore "github.com/kantaevsherhan/mini-ubuntu-server-panel/backend/internal/secrets"
	"github.com/kantaevsherhan/mini-ubuntu-server-panel/backend/internal/systemusers"
	telegramapi "github.com/kantaevsherhan/mini-ubuntu-server-panel/backend/internal/telegram"
	"gorm.io/gorm"
)

var usernamePattern = regexp.MustCompile(`^[a-z_][a-z0-9_-]{2,31}$`)

type API struct {
	DB          *gorm.DB
	SystemUsers systemusers.Client
	Secrets     secretstore.Writer
	Secret      string
	Version     string
}

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type createUserRequest struct {
	Username         string   `json:"username"`
	DisplayName      string   `json:"display_name"`
	Password         string   `json:"password"`
	Role             string   `json:"role"`
	SystemUsername   string   `json:"system_username"`
	CreatePanelUser  *bool    `json:"create_panel_user"`
	CreateSystemUser bool     `json:"create_system_user"`
	HomeDirectory    string   `json:"home_directory"`
	Shell            string   `json:"shell"`
	SystemGroups     []string `json:"system_groups"`
	AllowSudo        bool     `json:"allow_sudo"`
	CreateHome       bool     `json:"create_home"`
	AllowSSH         bool     `json:"allow_ssh"`
	SSHPublicKey     string   `json:"ssh_public_key"`
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
	secured.Post("/auth/password", a.changePassword)
	secured.Post("/auth/logout", a.logout)
	secured.Get("/auth/sessions", a.sessions)
	secured.Delete("/auth/sessions/:id", a.revokeSession)
	secured.Get("/dashboard", a.dashboard)
	secured.Get("/metrics/history", a.metricsHistory)
	secured.Get("/users", a.users)
	secured.Post("/users", a.requireRole("admin"), a.createUser)
	secured.Patch("/users/:id", a.requireRole("admin"), a.updateUser)
	secured.Delete("/users/:id", a.requireRole("admin"), a.deleteUser)
	secured.Post("/users/:id/reset-password", a.requireRole("admin"), a.resetUserPassword)
	secured.Get("/users/:id/sessions", a.requireRole("admin"), a.userSessions)
	secured.Get("/users/:id/system-details", a.requireRole("admin", "operator"), a.userSystemDetails)
	secured.Get("/system-users", a.requireRole("admin", "operator"), a.systemUsers)
	secured.Get("/telegram/settings", a.requireRole("admin"), a.telegramSettings)
	secured.Put("/telegram/settings", a.requireRole("admin"), a.updateTelegramSettings)
	secured.Put("/telegram/token", a.requireRole("admin"), a.updateTelegramToken)
	secured.Post("/telegram/check", a.requireRole("admin"), a.checkTelegram)
	secured.Get("/telegram/updates", a.requireRole("admin"), a.telegramUpdates)
	secured.Get("/telegram/recipients", a.requireRole("admin"), a.telegramRecipients)
	secured.Post("/telegram/recipients", a.requireRole("admin"), a.createTelegramRecipient)
	secured.Put("/telegram/recipients/:id", a.requireRole("admin"), a.updateTelegramRecipient)
	secured.Delete("/telegram/recipients/:id", a.requireRole("admin"), a.deleteTelegramRecipient)
	secured.Post("/telegram/recipients/:id/test", a.requireRole("admin"), a.testTelegramRecipient)
	secured.Get("/audit", a.requireRole("admin"), a.audit)
}

func (a API) health(c *fiber.Ctx) error {
	ctx, cancel := context.WithTimeout(c.UserContext(), 2*time.Second)
	defer cancel()
	sqlDB, err := a.DB.DB()
	if err != nil || sqlDB.PingContext(ctx) != nil {
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

	var user database.User
	err := a.DB.WithContext(c.UserContext()).Where("username = ?", request.Username).First(&user).Error
	if err != nil || !user.IsActive || !auth.Verify(user.PasswordHash, request.Password) {
		return invalidCredentials(c, a.DB, request.Username)
	}

	token, sessionID, expiresAt, err := auth.Sign(a.Secret, user.ID, request.Username, user.Role)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	session := database.WebSession{ID: sessionID, UserID: user.ID, IPAddress: c.IP(), UserAgent: truncate(c.Get(fiber.HeaderUserAgent), 512), CreatedAt: now, LastSeenAt: now, ExpiresAt: expiresAt}
	if err := a.DB.WithContext(c.UserContext()).Create(&session).Error; err != nil {
		return err
	}
	_ = a.DB.WithContext(c.UserContext()).Model(&user).Updates(map[string]any{"last_login_at": now, "updated_at": now}).Error
	database.Audit(a.DB, user.ID, "auth.login", "user", strconv.FormatInt(user.ID, 10), "{}", c.IP())
	return c.JSON(fiber.Map{"access_token": token, "token_type": "Bearer", "must_change_password": user.MustChangePassword})
}

func invalidCredentials(c *fiber.Ctx, db *gorm.DB, username string) error {
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
	var user database.User
	var session database.WebSession
	if err := a.DB.WithContext(c.UserContext()).First(&user, claims.UserID).Error; err != nil || !user.IsActive {
		return fiber.ErrUnauthorized
	}
	if err := a.DB.WithContext(c.UserContext()).Where("id = ? AND user_id = ?", claims.ID, claims.UserID).First(&session).Error; err != nil || session.RevokedAt != nil || time.Now().UTC().After(session.ExpiresAt) {
		return fiber.ErrUnauthorized
	}
	claims.Role = user.Role
	c.Locals("claims", claims)
	c.Locals("must_change_password", user.MustChangePassword)
	if session.LastSeenAt.Before(time.Now().UTC().Add(-time.Minute)) {
		_ = a.DB.WithContext(c.UserContext()).Model(&session).Update("last_seen_at", time.Now().UTC()).Error
	}
	if user.MustChangePassword && c.Path() != "/api/v1/me" && c.Path() != "/api/v1/auth/password" && c.Path() != "/api/v1/auth/logout" {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "password_change_required"})
	}
	return c.Next()
}

func (a API) changePassword(c *fiber.Ctx) error {
	var request struct {
		CurrentPassword string `json:"current_password"`
		NewPassword     string `json:"new_password"`
	}
	if err := c.BodyParser(&request); err != nil || len(request.NewPassword) < 12 || len(request.NewPassword) > 1024 || request.CurrentPassword == request.NewPassword {
		return c.Status(fiber.StatusUnprocessableEntity).JSON(fiber.Map{"error": "password_validation_failed"})
	}
	claims := c.Locals("claims").(*auth.Claims)
	var user database.User
	if err := a.DB.WithContext(c.UserContext()).First(&user, claims.UserID).Error; err != nil || !auth.Verify(user.PasswordHash, request.CurrentPassword) {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "current_password_invalid"})
	}
	newHash, err := auth.Hash(request.NewPassword)
	if err != nil {
		return err
	}
	err = a.DB.WithContext(c.UserContext()).Transaction(func(tx *gorm.DB) error {
		now := time.Now().UTC()
		if err := tx.Model(&database.User{}).Where("id = ?", claims.UserID).Updates(map[string]any{"password_hash": newHash, "must_change_password": false, "updated_at": now}).Error; err != nil {
			return err
		}
		return tx.Model(&database.WebSession{}).Where("user_id = ? AND id <> ? AND revoked_at IS NULL", claims.UserID, claims.ID).Update("revoked_at", now).Error
	})
	if err != nil {
		return err
	}
	database.Audit(a.DB, claims.UserID, "auth.password.change", "user", strconv.FormatInt(claims.UserID, 10), `{"other_sessions_revoked":true}`, c.IP())
	return c.SendStatus(fiber.StatusNoContent)
}

func (a API) logout(c *fiber.Ctx) error {
	claims := c.Locals("claims").(*auth.Claims)
	err := a.DB.WithContext(c.UserContext()).Model(&database.WebSession{}).Where("id = ? AND revoked_at IS NULL", claims.ID).Update("revoked_at", time.Now().UTC()).Error
	if err != nil {
		return err
	}
	return c.SendStatus(fiber.StatusNoContent)
}

func (a API) sessions(c *fiber.Ctx) error {
	claims := c.Locals("claims").(*auth.Claims)
	var sessions []database.WebSession
	err := a.DB.WithContext(c.UserContext()).Where("user_id = ? AND revoked_at IS NULL AND expires_at > ?", claims.UserID, time.Now().UTC()).Order("last_seen_at DESC").Find(&sessions).Error
	if err != nil {
		return err
	}
	result := make([]fiber.Map, 0)
	for _, session := range sessions {
		result = append(result, fiber.Map{"id": session.ID, "ip_address": session.IPAddress, "user_agent": session.UserAgent, "created_at": session.CreatedAt, "last_seen_at": session.LastSeenAt, "expires_at": session.ExpiresAt, "current": session.ID == claims.ID})
	}
	return c.JSON(result)
}

func (a API) revokeSession(c *fiber.Ctx) error {
	claims := c.Locals("claims").(*auth.Claims)
	sessionID := c.Params("id")
	if sessionID == "" || len(sessionID) > 64 {
		return fiber.ErrBadRequest
	}
	result := a.DB.WithContext(c.UserContext()).Model(&database.WebSession{}).Where("id = ? AND user_id = ? AND revoked_at IS NULL", sessionID, claims.UserID).Update("revoked_at", time.Now().UTC())
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return fiber.ErrNotFound
	}
	return c.SendStatus(fiber.StatusNoContent)
}

func truncate(value string, maximum int) string {
	if len(value) <= maximum {
		return value
	}
	return value[:maximum]
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
	var userCount, eventCount int64
	if err := a.DB.WithContext(c.UserContext()).Model(&database.User{}).Where("is_active = ?", true).Count(&userCount).Error; err != nil {
		return err
	}
	if err := a.DB.WithContext(c.UserContext()).Model(&database.NotificationEvent{}).Where("status = ?", "pending").Count(&eventCount).Error; err != nil {
		return err
	}
	users, events = int(userCount), int(eventCount)
	hostname, _ := os.Hostname()
	return c.JSON(fiber.Map{"hostname": hostname, "panel_users": users, "pending_notifications": events, "status": "online"})
}

func (a API) metricsHistory(c *fiber.Ctx) error {
	rangeName := c.Query("range", "day")
	now := time.Now().UTC()
	var start time.Time
	var bucketSeconds int64
	switch rangeName {
	case "day":
		start = now.Add(-24 * time.Hour)
		bucketSeconds = 60
	case "week":
		start = now.Add(-7 * 24 * time.Hour)
		bucketSeconds = 15 * 60
	case "month":
		start = now.Add(-30 * 24 * time.Hour)
		bucketSeconds = 60 * 60
	case "all":
		var sample database.MetricSample
		err := a.DB.WithContext(c.UserContext()).Order("sampled_at").First(&sample).Error
		if err == nil {
			start = sample.SampledAt
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		} else {
			start = now
		}
		span := now.Sub(start)
		bucketSeconds = maxInt64(60, int64(span.Seconds()/500))
	default:
		return c.Status(fiber.StatusUnprocessableEntity).JSON(fiber.Map{"error": "metrics_range_invalid"})
	}
	type metricAggregate struct {
		SampledAt        string
		CPUPercent       float64
		MemoryPercent    float64
		MemoryUsedBytes  float64
		MemoryTotalBytes float64
	}
	var aggregates []metricAggregate
	bucketExpr := fmt.Sprintf("CAST(strftime('%%s', sampled_at) AS INTEGER) / %d", bucketSeconds)
	err := a.DB.WithContext(c.UserContext()).Model(&database.MetricSample{}).Select("datetime((CAST(strftime('%s', sampled_at) AS INTEGER) / ?) * ?, 'unixepoch') AS sampled_at, AVG(cpu_percent) AS cpu_percent, AVG(memory_percent) AS memory_percent, AVG(memory_used_bytes) AS memory_used_bytes, MAX(memory_total_bytes) AS memory_total_bytes", bucketSeconds, bucketSeconds).Where("sampled_at >= ?", start).Group(bucketExpr).Order("sampled_at ASC").Limit(1000).Scan(&aggregates).Error
	if err != nil {
		return err
	}
	points := make([]fiber.Map, 0)
	for _, aggregate := range aggregates {
		points = append(points, fiber.Map{"sampled_at": aggregate.SampledAt + "Z", "cpu_percent": aggregate.CPUPercent, "memory_percent": aggregate.MemoryPercent, "memory_used_bytes": uint64(aggregate.MemoryUsedBytes), "memory_total_bytes": uint64(aggregate.MemoryTotalBytes)})
	}
	return c.JSON(fiber.Map{"range": rangeName, "points": points})
}

func maxInt64(left, right int64) int64 {
	if left > right {
		return left
	}
	return right
}

func (a API) users(c *fiber.Ctx) error {
	var users []database.User
	err := a.DB.WithContext(c.UserContext()).Order("username").Find(&users).Error
	if err != nil {
		return err
	}
	result := make([]fiber.Map, 0)
	for _, user := range users {
		result = append(result, fiber.Map{"id": user.ID, "username": user.Username, "display_name": user.DisplayName, "role": user.Role, "is_active": user.IsActive, "system_username": user.SystemUsername, "created_at": user.CreatedAt, "last_login_at": user.LastLoginAt})
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
	createPanel := request.CreatePanelUser == nil || *request.CreatePanelUser
	if !createPanel && !request.CreateSystemUser {
		return c.Status(fiber.StatusUnprocessableEntity).JSON(fiber.Map{"error": "user_target_required"})
	}
	if !usernamePattern.MatchString(request.Username) || len(request.DisplayName) > 128 {
		return c.Status(fiber.StatusUnprocessableEntity).JSON(fiber.Map{"error": "user_validation_failed"})
	}
	if createPanel && (len(request.Password) < 12 || len(request.Password) > 1024) {
		return c.Status(fiber.StatusUnprocessableEntity).JSON(fiber.Map{"error": "password_validation_failed"})
	}
	if request.SystemUsername == "" {
		request.SystemUsername = request.Username
	}
	if !usernamePattern.MatchString(request.SystemUsername) {
		return c.Status(fiber.StatusUnprocessableEntity).JSON(fiber.Map{"error": "system_username_invalid"})
	}
	if createPanel && request.Role != "admin" && request.Role != "operator" && request.Role != "viewer" {
		return c.Status(fiber.StatusUnprocessableEntity).JSON(fiber.Map{"error": "role_invalid"})
	}
	if request.CreateSystemUser {
		if a.SystemUsers == nil {
			return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"error": "system_user_service_unavailable"})
		}
		exists, err := a.SystemUsers.Exists(request.SystemUsername)
		if err != nil {
			return err
		}
		if exists {
			return c.Status(fiber.StatusConflict).JSON(fiber.Map{"error": "system_username_exists"})
		}
	}
	if createPanel {
		var count int64
		if err := a.DB.WithContext(c.UserContext()).Model(&database.User{}).Where("username = ?", request.Username).Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			return c.Status(fiber.StatusConflict).JSON(fiber.Map{"error": "username_exists"})
		}
	}

	systemCreated := false
	if request.CreateSystemUser {
		if request.HomeDirectory == "" {
			request.HomeDirectory = "/home/" + request.SystemUsername
		}
		if request.Shell == "" {
			request.Shell = "/bin/bash"
		}
		operation := systemusers.CreateRequest{
			Username: request.SystemUsername, HomeDirectory: request.HomeDirectory, Shell: request.Shell,
			Groups: request.SystemGroups, AllowSudo: request.AllowSudo, CreateHome: request.CreateHome,
			AllowSSH: request.AllowSSH, SSHPublicKey: request.SSHPublicKey,
		}
		if err := a.SystemUsers.Create(c.UserContext(), operation); err != nil {
			return c.Status(fiber.StatusUnprocessableEntity).JSON(fiber.Map{"error": "system_user_create_failed"})
		}
		systemCreated = true
	}

	var user database.User
	if createPanel {
		hash, err := auth.Hash(request.Password)
		if err != nil {
			if systemCreated {
				_ = a.SystemUsers.Delete(c.UserContext(), systemusers.DeleteRequest{Username: request.SystemUsername, DeleteUser: true, RemoveHome: request.CreateHome})
			}
			return err
		}
		now := time.Now().UTC()
		var systemUsername *string
		if request.CreateSystemUser || request.SystemUsername != request.Username {
			systemUsername = &request.SystemUsername
		}
		user = database.User{Username: request.Username, DisplayName: request.DisplayName, PasswordHash: hash, Role: request.Role, IsActive: true, SystemUsername: systemUsername, CreatedAt: now, UpdatedAt: now}
		if err := a.DB.WithContext(c.UserContext()).Create(&user).Error; err != nil {
			if systemCreated {
				if rollbackErr := a.SystemUsers.Delete(c.UserContext(), systemusers.DeleteRequest{Username: request.SystemUsername, DeleteUser: true, RemoveHome: request.CreateHome}); rollbackErr != nil {
					return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "panel_user_create_failed_rollback_failed"})
				}
			}
			return c.Status(fiber.StatusConflict).JSON(fiber.Map{"error": "username_exists"})
		}
	}
	claims := c.Locals("claims").(*auth.Claims)
	details := fmt.Sprintf(`{"password":"hidden","panel_user":%t,"system_user":%t,"system_username":%q}`, createPanel, systemCreated, request.SystemUsername)
	targetID := request.SystemUsername
	if createPanel {
		targetID = fmt.Sprint(user.ID)
	}
	database.Audit(a.DB, claims.UserID, "user.create", "user", targetID, details, c.IP())
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"id": user.ID, "panel_user_created": createPanel, "system_user_created": systemCreated})
}

func (a API) updateUser(c *fiber.Ctx) error {
	targetID, err := parseID(c.Params("id"))
	if err != nil {
		return fiber.ErrBadRequest
	}
	var request struct {
		DisplayName    string `json:"display_name"`
		Role           string `json:"role"`
		Active         bool   `json:"is_active"`
		SystemUsername string `json:"system_username"`
	}
	if err := c.BodyParser(&request); err != nil {
		return fiber.ErrBadRequest
	}
	request.DisplayName = strings.TrimSpace(request.DisplayName)
	request.SystemUsername = strings.TrimSpace(request.SystemUsername)
	if len(request.DisplayName) > 128 || (request.Role != "admin" && request.Role != "operator" && request.Role != "viewer") || (request.SystemUsername != "" && !usernamePattern.MatchString(request.SystemUsername)) {
		return c.Status(fiber.StatusUnprocessableEntity).JSON(fiber.Map{"error": "user_validation_failed"})
	}
	claims := c.Locals("claims").(*auth.Claims)
	if targetID == claims.UserID && (request.Role != "admin" || !request.Active) {
		return c.Status(fiber.StatusConflict).JSON(fiber.Map{"error": "cannot_demote_or_disable_self"})
	}
	var target database.User
	if err := a.DB.WithContext(c.UserContext()).First(&target, targetID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return fiber.ErrNotFound
		}
		return err
	}
	if target.Role == "admin" && target.IsActive && (request.Role != "admin" || !request.Active) {
		if last, err := a.isLastActiveAdmin(c.UserContext(), targetID); err != nil {
			return err
		} else if last {
			return c.Status(fiber.StatusConflict).JSON(fiber.Map{"error": "last_admin_protected"})
		}
	}
	var systemUsername *string
	if request.SystemUsername != "" {
		systemUsername = &request.SystemUsername
	}
	err = a.DB.WithContext(c.UserContext()).Transaction(func(tx *gorm.DB) error {
		now := time.Now().UTC()
		if err := tx.Model(&target).Updates(map[string]any{"display_name": request.DisplayName, "role": request.Role, "is_active": request.Active, "system_username": systemUsername, "updated_at": now}).Error; err != nil {
			return err
		}
		if !request.Active {
			return tx.Model(&database.WebSession{}).Where("user_id = ? AND revoked_at IS NULL", targetID).Update("revoked_at", now).Error
		}
		return nil
	})
	if err != nil {
		return err
	}
	database.Audit(a.DB, claims.UserID, "user.update", "user", strconv.FormatInt(targetID, 10), fmt.Sprintf(`{"role":%q,"active":%t}`, request.Role, request.Active), c.IP())
	return c.SendStatus(fiber.StatusNoContent)
}

func (a API) deleteUser(c *fiber.Ctx) error {
	targetID, err := parseID(c.Params("id"))
	if err != nil {
		return fiber.ErrBadRequest
	}
	claims := c.Locals("claims").(*auth.Claims)
	var target database.User
	if err := a.DB.WithContext(c.UserContext()).First(&target, targetID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return fiber.ErrNotFound
		}
		return err
	}
	var request struct {
		DeletePanelUser   *bool `json:"delete_panel_user"`
		DeleteSystemUser  bool  `json:"delete_system_user"`
		DeleteHome        bool  `json:"delete_home_directory"`
		DeleteSSHKeys     bool  `json:"delete_ssh_keys"`
		TerminateSessions bool  `json:"terminate_sessions"`
	}
	if len(c.Body()) > 0 {
		if err := c.BodyParser(&request); err != nil {
			return fiber.ErrBadRequest
		}
	}
	deletePanel := request.DeletePanelUser == nil || *request.DeletePanelUser
	if request.DeleteHome && !request.DeleteSystemUser {
		return c.Status(fiber.StatusUnprocessableEntity).JSON(fiber.Map{"error": "home_delete_requires_system_user_delete"})
	}
	if !deletePanel && !request.DeleteSystemUser && !request.DeleteSSHKeys && !request.TerminateSessions {
		return c.Status(fiber.StatusUnprocessableEntity).JSON(fiber.Map{"error": "user_delete_target_required"})
	}
	if deletePanel && targetID == claims.UserID {
		return c.Status(fiber.StatusConflict).JSON(fiber.Map{"error": "cannot_delete_self"})
	}
	if deletePanel && target.Role == "admin" && target.IsActive {
		if last, err := a.isLastActiveAdmin(c.UserContext(), targetID); err != nil {
			return err
		} else if last {
			return c.Status(fiber.StatusConflict).JSON(fiber.Map{"error": "last_admin_protected"})
		}
	}
	if (request.DeleteSystemUser || request.DeleteSSHKeys) && target.SystemUsername == nil {
		return c.Status(fiber.StatusConflict).JSON(fiber.Map{"error": "system_user_not_linked"})
	}
	needsSystemHelper := request.DeleteSystemUser || request.DeleteSSHKeys || (request.TerminateSessions && target.SystemUsername != nil)
	if needsSystemHelper && a.SystemUsers == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"error": "system_user_service_unavailable"})
	}

	var sessions []database.WebSession
	if deletePanel || request.TerminateSessions {
		if err := a.DB.WithContext(c.UserContext()).Where("user_id = ?", target.ID).Find(&sessions).Error; err != nil {
			return err
		}
	}
	if deletePanel {
		if err := a.DB.WithContext(c.UserContext()).Delete(&target).Error; err != nil {
			return err
		}
	} else if request.TerminateSessions {
		now := time.Now().UTC()
		if err := a.DB.WithContext(c.UserContext()).Model(&database.WebSession{}).Where("user_id = ? AND revoked_at IS NULL", target.ID).Update("revoked_at", now).Error; err != nil {
			return err
		}
	}

	if target.SystemUsername != nil && needsSystemHelper {
		systemRequest := systemusers.DeleteRequest{
			Username: *target.SystemUsername, DeleteUser: request.DeleteSystemUser, RemoveHome: request.DeleteHome,
			RemoveSSHKeys: request.DeleteSSHKeys, TerminateSessions: request.TerminateSessions,
		}
		if err := a.SystemUsers.Delete(c.UserContext(), systemRequest); err != nil {
			if deletePanel {
				if rollbackErr := restorePanelUser(a.DB.WithContext(c.UserContext()), target, sessions); rollbackErr != nil {
					return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "system_user_delete_failed_rollback_failed"})
				}
			} else if request.TerminateSessions {
				if rollbackErr := restoreSessions(a.DB.WithContext(c.UserContext()), sessions); rollbackErr != nil {
					return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "system_user_delete_failed_rollback_failed"})
				}
			}
			return c.Status(fiber.StatusConflict).JSON(fiber.Map{"error": "system_user_delete_failed"})
		}
	}
	details := fmt.Sprintf(`{"panel_user":%t,"system_user":%t,"home":%t,"ssh_keys":%t,"sessions":%t}`, deletePanel, request.DeleteSystemUser, request.DeleteHome, request.DeleteSSHKeys, request.TerminateSessions)
	database.Audit(a.DB, claims.UserID, "user.delete", "user", strconv.FormatInt(targetID, 10), details, c.IP())
	return c.SendStatus(fiber.StatusNoContent)
}

func restoreSessions(db *gorm.DB, sessions []database.WebSession) error {
	return db.Transaction(func(tx *gorm.DB) error {
		for index := range sessions {
			if err := tx.Save(&sessions[index]).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func restorePanelUser(db *gorm.DB, target database.User, sessions []database.WebSession) error {
	return db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&target).Error; err != nil {
			return err
		}
		if len(sessions) > 0 {
			return tx.Create(&sessions).Error
		}
		return nil
	})
}

func (a API) resetUserPassword(c *fiber.Ctx) error {
	targetID, err := parseID(c.Params("id"))
	if err != nil {
		return fiber.ErrBadRequest
	}
	var request struct {
		Password string `json:"password"`
	}
	if err := c.BodyParser(&request); err != nil || len(request.Password) < 12 || len(request.Password) > 1024 {
		return c.Status(fiber.StatusUnprocessableEntity).JSON(fiber.Map{"error": "password_validation_failed"})
	}
	hash, err := auth.Hash(request.Password)
	if err != nil {
		return err
	}
	var target database.User
	if err := a.DB.WithContext(c.UserContext()).First(&target, targetID).Error; errors.Is(err, gorm.ErrRecordNotFound) {
		return fiber.ErrNotFound
	} else if err != nil {
		return err
	}
	err = a.DB.WithContext(c.UserContext()).Transaction(func(tx *gorm.DB) error {
		now := time.Now().UTC()
		if err := tx.Model(&target).Updates(map[string]any{"password_hash": hash, "must_change_password": true, "updated_at": now}).Error; err != nil {
			return err
		}
		return tx.Model(&database.WebSession{}).Where("user_id = ? AND revoked_at IS NULL", targetID).Update("revoked_at", now).Error
	})
	if err != nil {
		return err
	}
	claims := c.Locals("claims").(*auth.Claims)
	database.Audit(a.DB, claims.UserID, "user.password.reset", "user", strconv.FormatInt(targetID, 10), `{"password":"hidden","sessions_revoked":true}`, c.IP())
	return c.SendStatus(fiber.StatusNoContent)
}

func (a API) userSessions(c *fiber.Ctx) error {
	targetID, err := parseID(c.Params("id"))
	if err != nil {
		return fiber.ErrBadRequest
	}
	var sessions []database.WebSession
	err = a.DB.WithContext(c.UserContext()).Where("user_id = ?", targetID).Order("last_seen_at DESC").Limit(100).Find(&sessions).Error
	if err != nil {
		return err
	}
	result := make([]fiber.Map, 0)
	for _, session := range sessions {
		result = append(result, fiber.Map{"id": session.ID, "ip_address": session.IPAddress, "user_agent": session.UserAgent, "created_at": session.CreatedAt, "last_seen_at": session.LastSeenAt, "expires_at": session.ExpiresAt, "revoked_at": session.RevokedAt})
	}
	return c.JSON(result)
}

func (a API) userSystemDetails(c *fiber.Ctx) error {
	targetID, err := parseID(c.Params("id"))
	if err != nil {
		return fiber.ErrBadRequest
	}
	var target database.User
	if err := a.DB.WithContext(c.UserContext()).First(&target, targetID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return fiber.ErrNotFound
		}
		return err
	}
	if target.SystemUsername == nil {
		return c.Status(fiber.StatusConflict).JSON(fiber.Map{"error": "system_user_not_linked"})
	}
	details, err := systemusers.Get(*target.SystemUsername)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "system_user_not_found"})
	}
	return c.JSON(details)
}

func (a API) isLastActiveAdmin(ctx context.Context, excludedID int64) (bool, error) {
	var count int64
	err := a.DB.WithContext(ctx).Model(&database.User{}).Where("role = ? AND is_active = ? AND id <> ?", "admin", true, excludedID).Count(&count).Error
	return count == 0, err
}

func parseID(value string) (int64, error) {
	id, err := strconv.ParseInt(value, 10, 64)
	if err != nil || id <= 0 {
		return 0, fmt.Errorf("invalid id")
	}
	return id, nil
}

func (a API) systemUsers(c *fiber.Ctx) error {
	users, err := systemusers.List()
	if err != nil {
		return err
	}
	return c.JSON(users)
}

func (a API) telegramSettings(c *fiber.Ctx) error {
	var settings database.TelegramSetting
	if err := a.DB.WithContext(c.UserContext()).First(&settings, 1).Error; err != nil {
		return err
	}
	var recipientCount int64
	if err := a.DB.WithContext(c.UserContext()).Model(&database.TelegramRecipient{}).Where("enabled = ?", true).Count(&recipientCount).Error; err != nil {
		return err
	}
	return c.JSON(fiber.Map{"enabled": settings.Enabled, "api_base_url": settings.APIBaseURL, "request_timeout_seconds": settings.RequestTimeoutSeconds, "retry_count": settings.RetryCount, "recipient_count": recipientCount, "token_configured": secretstore.TelegramToken(secretstore.DefaultPath) != ""})
}

func (a API) updateTelegramToken(c *fiber.Ctx) error {
	if a.Secrets == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"error": "secret_writer_unavailable"})
	}
	var request struct {
		Token string `json:"token"`
	}
	if err := c.BodyParser(&request); err != nil || secretstore.ValidateTelegramToken(request.Token) != nil {
		return c.Status(fiber.StatusUnprocessableEntity).JSON(fiber.Map{"error": "telegram_token_invalid"})
	}
	if err := a.Secrets.SetTelegramToken(c.UserContext(), request.Token); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "telegram_token_update_failed"})
	}
	claims := c.Locals("claims").(*auth.Claims)
	database.Audit(a.DB, claims.UserID, "telegram.token.update", "telegram", "1", `{"token_changed":true,"token_value":"hidden"}`, c.IP())
	return c.SendStatus(fiber.StatusNoContent)
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
	err := a.DB.WithContext(c.UserContext()).Model(&database.TelegramSetting{}).Where("id = ?", 1).Updates(map[string]any{"enabled": request.Enabled, "api_base_url": request.APIBaseURL, "request_timeout_seconds": request.RequestTimeout, "retry_count": request.RetryCount, "updated_at": time.Now().UTC()}).Error
	if err != nil {
		return err
	}
	claims := c.Locals("claims").(*auth.Claims)
	database.Audit(a.DB, claims.UserID, "telegram.settings.update", "telegram", "1", `{"token_value":"hidden"}`, c.IP())
	return c.SendStatus(fiber.StatusNoContent)
}

type telegramRecipientRequest struct {
	TelegramUserID *int64 `json:"telegram_user_id"`
	TelegramChatID int64  `json:"telegram_chat_id"`
	DisplayName    string `json:"display_name"`
	Enabled        bool   `json:"enabled"`
	ReceiveAlerts  bool   `json:"receive_alerts"`
	ReceiveAudit   bool   `json:"receive_audit"`
	ReceiveUpdates bool   `json:"receive_updates"`
}

func (a API) telegramClient(ctx context.Context) (*telegramapi.Client, error) {
	var settings database.TelegramSetting
	if err := a.DB.WithContext(ctx).First(&settings, 1).Error; err != nil {
		return nil, err
	}
	return telegramapi.New(settings.APIBaseURL, secretstore.TelegramToken(secretstore.DefaultPath), time.Duration(settings.RequestTimeoutSeconds)*time.Second)
}

func (a API) checkTelegram(c *fiber.Ctx) error {
	client, err := a.telegramClient(c.UserContext())
	if err != nil {
		return c.Status(fiber.StatusUnprocessableEntity).JSON(fiber.Map{"error": "telegram_token_not_configured"})
	}
	bot, err := client.GetMe(c.UserContext())
	if err != nil {
		return c.Status(fiber.StatusBadGateway).JSON(fiber.Map{"error": "telegram_connection_failed"})
	}
	var recipients int64
	_ = a.DB.WithContext(c.UserContext()).Model(&database.TelegramRecipient{}).Where("enabled = ?", true).Count(&recipients).Error
	return c.JSON(fiber.Map{"bot_id": bot.ID, "username": bot.Username, "recipients": recipients})
}

func (a API) telegramUpdates(c *fiber.Ctx) error {
	client, err := a.telegramClient(c.UserContext())
	if err != nil {
		return c.Status(fiber.StatusUnprocessableEntity).JSON(fiber.Map{"error": "telegram_token_not_configured"})
	}
	updates, err := client.GetUpdates(c.UserContext())
	if err != nil {
		return c.Status(fiber.StatusBadGateway).JSON(fiber.Map{"error": "telegram_updates_failed"})
	}
	candidates := make([]fiber.Map, 0)
	seen := map[int64]bool{}
	for _, update := range updates {
		if update.Message == nil || seen[update.Message.Chat.ID] {
			continue
		}
		seen[update.Message.Chat.ID] = true
		candidates = append(candidates, fiber.Map{"telegram_user_id": update.Message.From.ID, "telegram_chat_id": update.Message.Chat.ID, "username": update.Message.From.Username, "display_name": strings.TrimSpace(update.Message.From.FirstName + " " + update.Message.Chat.Title), "chat_type": update.Message.Chat.Type})
	}
	return c.JSON(candidates)
}

func (a API) telegramRecipients(c *fiber.Ctx) error {
	var recipients []database.TelegramRecipient
	err := a.DB.WithContext(c.UserContext()).Order("display_name,id").Find(&recipients).Error
	if err != nil {
		return err
	}
	result := make([]fiber.Map, 0)
	for _, recipient := range recipients {
		result = append(result, fiber.Map{"id": recipient.ID, "telegram_user_id": recipient.TelegramUserID, "telegram_chat_id": recipient.TelegramChatID, "display_name": recipient.DisplayName, "enabled": recipient.Enabled, "receive_alerts": recipient.ReceiveAlerts, "receive_audit": recipient.ReceiveAudit, "receive_updates": recipient.ReceiveUpdates, "created_at": recipient.CreatedAt})
	}
	return c.JSON(result)
}

func (a API) createTelegramRecipient(c *fiber.Ctx) error {
	var request telegramRecipientRequest
	if err := c.BodyParser(&request); err != nil || !validTelegramRecipient(request) {
		return c.Status(fiber.StatusUnprocessableEntity).JSON(fiber.Map{"error": "telegram_recipient_invalid"})
	}
	name := strings.TrimSpace(request.DisplayName)
	recipient := database.TelegramRecipient{TelegramUserID: request.TelegramUserID, TelegramChatID: request.TelegramChatID, DisplayName: &name, Enabled: request.Enabled, ReceiveAlerts: request.ReceiveAlerts, ReceiveAudit: request.ReceiveAudit, ReceiveUpdates: request.ReceiveUpdates, CreatedAt: time.Now().UTC()}
	if err := a.DB.WithContext(c.UserContext()).Create(&recipient).Error; err != nil {
		return c.Status(fiber.StatusConflict).JSON(fiber.Map{"error": "telegram_recipient_exists"})
	}
	claims := c.Locals("claims").(*auth.Claims)
	database.Audit(a.DB, claims.UserID, "telegram.recipient.create", "telegram_recipient", strconv.FormatInt(recipient.ID, 10), "{}", c.IP())
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"id": recipient.ID})
}

func (a API) updateTelegramRecipient(c *fiber.Ctx) error {
	id, err := parseID(c.Params("id"))
	if err != nil {
		return fiber.ErrBadRequest
	}
	var request telegramRecipientRequest
	if err := c.BodyParser(&request); err != nil || !validTelegramRecipient(request) {
		return c.Status(fiber.StatusUnprocessableEntity).JSON(fiber.Map{"error": "telegram_recipient_invalid"})
	}
	name := strings.TrimSpace(request.DisplayName)
	result := a.DB.WithContext(c.UserContext()).Model(&database.TelegramRecipient{}).Where("id = ?", id).Updates(map[string]any{"telegram_user_id": request.TelegramUserID, "telegram_chat_id": request.TelegramChatID, "display_name": name, "enabled": request.Enabled, "receive_alerts": request.ReceiveAlerts, "receive_audit": request.ReceiveAudit, "receive_updates": request.ReceiveUpdates})
	if result.Error != nil {
		return c.Status(fiber.StatusConflict).JSON(fiber.Map{"error": "telegram_recipient_exists"})
	}
	if result.RowsAffected == 0 {
		return fiber.ErrNotFound
	}
	claims := c.Locals("claims").(*auth.Claims)
	database.Audit(a.DB, claims.UserID, "telegram.recipient.update", "telegram_recipient", strconv.FormatInt(id, 10), "{}", c.IP())
	return c.SendStatus(fiber.StatusNoContent)
}

func (a API) deleteTelegramRecipient(c *fiber.Ctx) error {
	id, err := parseID(c.Params("id"))
	if err != nil {
		return fiber.ErrBadRequest
	}
	result := a.DB.WithContext(c.UserContext()).Delete(&database.TelegramRecipient{}, id)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return fiber.ErrNotFound
	}
	claims := c.Locals("claims").(*auth.Claims)
	database.Audit(a.DB, claims.UserID, "telegram.recipient.delete", "telegram_recipient", strconv.FormatInt(id, 10), "{}", c.IP())
	return c.SendStatus(fiber.StatusNoContent)
}

func (a API) testTelegramRecipient(c *fiber.Ctx) error {
	id, err := parseID(c.Params("id"))
	if err != nil {
		return fiber.ErrBadRequest
	}
	var recipient database.TelegramRecipient
	if err := a.DB.WithContext(c.UserContext()).First(&recipient, id).Error; err != nil {
		return fiber.ErrNotFound
	}
	client, err := a.telegramClient(c.UserContext())
	if err != nil {
		return c.Status(fiber.StatusUnprocessableEntity).JSON(fiber.Map{"error": "telegram_token_not_configured"})
	}
	hostname, _ := os.Hostname()
	message := fmt.Sprintf("✅ Mini Ubuntu Server Panel connected\n\nServer: %s\nTime: %s", hostname, time.Now().Format(time.RFC3339))
	if err := client.SendMessage(c.UserContext(), recipient.TelegramChatID, message); err != nil {
		return c.Status(fiber.StatusBadGateway).JSON(fiber.Map{"error": "telegram_send_failed"})
	}
	claims := c.Locals("claims").(*auth.Claims)
	database.Audit(a.DB, claims.UserID, "telegram.test.send", "telegram_recipient", strconv.FormatInt(id, 10), "{}", c.IP())
	return c.SendStatus(fiber.StatusNoContent)
}

func validTelegramRecipient(request telegramRecipientRequest) bool {
	return request.TelegramChatID != 0 && len(strings.TrimSpace(request.DisplayName)) <= 128
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
	var events []database.AuditEvent
	err := a.DB.WithContext(c.UserContext()).Order("id DESC").Limit(200).Find(&events).Error
	if err != nil {
		return err
	}
	result := make([]fiber.Map, 0)
	for _, event := range events {
		result = append(result, fiber.Map{"id": event.ID, "action": event.Action, "target_type": event.TargetType, "target_id": event.TargetID, "details": event.DetailsJSON, "created_at": event.CreatedAt})
	}
	return c.JSON(result)
}
