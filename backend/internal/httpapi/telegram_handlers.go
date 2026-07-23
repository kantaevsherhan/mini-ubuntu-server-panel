package httpapi

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/kantaevsherhan/mini-ubuntu-server-panel/backend/internal/auth"
	"github.com/kantaevsherhan/mini-ubuntu-server-panel/backend/internal/database"
	secretstore "github.com/kantaevsherhan/mini-ubuntu-server-panel/backend/internal/secrets"
	telegramapi "github.com/kantaevsherhan/mini-ubuntu-server-panel/backend/internal/telegram"
)

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
