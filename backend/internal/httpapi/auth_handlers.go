package httpapi

import (
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/kantaevsherhan/mini-ubuntu-server-panel/backend/internal/auth"
	"github.com/kantaevsherhan/mini-ubuntu-server-panel/backend/internal/database"
	"gorm.io/gorm"
)

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
	return c.JSON(fiber.Map{"access_token": token, "token_type": "Bearer", "must_change_password": user.MustChangePassword, "username": user.Username, "role": user.Role})
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
