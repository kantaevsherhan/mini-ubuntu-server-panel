package httpapi

import (
	"context"
	"regexp"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/limiter"
	"github.com/kantaevsherhan/mini-ubuntu-server-panel/backend/internal/database"
	dockermanager "github.com/kantaevsherhan/mini-ubuntu-server-panel/backend/internal/docker"
	"github.com/kantaevsherhan/mini-ubuntu-server-panel/backend/internal/firewall"
	"github.com/kantaevsherhan/mini-ubuntu-server-panel/backend/internal/processes"
	secretstore "github.com/kantaevsherhan/mini-ubuntu-server-panel/backend/internal/secrets"
	"github.com/kantaevsherhan/mini-ubuntu-server-panel/backend/internal/services"
	"github.com/kantaevsherhan/mini-ubuntu-server-panel/backend/internal/systemusers"
	"gorm.io/gorm"
)

var usernamePattern = regexp.MustCompile(`^[a-z_][a-z0-9_-]{2,31}$`)

type API struct {
	DB          *gorm.DB
	SystemUsers systemusers.Client
	Secrets     secretstore.Writer
	Processes   processes.Controller
	Services    services.Controller
	Docker      dockermanager.Controller
	Firewall    firewall.Controller
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
	secured.Get("/processes", a.processList)
	secured.Post("/processes/:pid/signal", a.requireRole("admin", "operator"), a.processSignal)
	secured.Get("/services", a.requireRole("admin", "operator"), a.serviceList)
	secured.Post("/services/:unit/action", a.requireRole("admin", "operator"), a.serviceAction)
	secured.Get("/docker/containers", a.requireRole("admin", "operator"), a.dockerContainers)
	secured.Post("/docker/containers/:id/action", a.requireRole("admin", "operator"), a.dockerContainerAction)
	secured.Get("/firewall", a.requireRole("admin", "operator"), a.firewallStatus)
	secured.Post("/firewall/rules", a.requireRole("admin"), a.firewallAddRule)
	secured.Delete("/firewall/rules/:number", a.requireRole("admin"), a.firewallDeleteRule)
	secured.Get("/users", a.requireRole("admin", "operator"), a.users)
	secured.Post("/users", a.requireRole("admin"), a.createUser)
	secured.Patch("/users/:id", a.requireRole("admin"), a.updateUser)
	secured.Delete("/users/:id", a.requireRole("admin"), a.deleteUser)
	secured.Post("/users/:id/reset-password", a.requireRole("admin"), a.resetUserPassword)
	secured.Get("/users/:id/sessions", a.requireRole("admin", "operator"), a.userSessions)
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
	secured.Get("/notifications/rules", a.requireRole("admin"), a.notificationRules)
	secured.Put("/notifications/rules/:key", a.requireRole("admin"), a.updateNotificationRule)
	secured.Get("/notifications/history", a.requireRole("admin", "operator"), a.notificationHistory)
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

func truncate(value string, maximum int) string {
	if len(value) <= maximum {
		return value
	}
	return value[:maximum]
}
