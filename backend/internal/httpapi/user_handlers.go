package httpapi

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/kantaevsherhan/mini-ubuntu-server-panel/backend/internal/auth"
	"github.com/kantaevsherhan/mini-ubuntu-server-panel/backend/internal/database"
	"github.com/kantaevsherhan/mini-ubuntu-server-panel/backend/internal/systemusers"
	"gorm.io/gorm"
)

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
