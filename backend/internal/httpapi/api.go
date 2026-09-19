package httpapi

import (
	"context"
	"regexp"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/limiter"
	"github.com/kantaevsherhan/mini-ubuntu-server-panel/backend/internal/database"
	dockermanager "github.com/kantaevsherhan/mini-ubuntu-server-panel/backend/internal/docker"
	filemanager "github.com/kantaevsherhan/mini-ubuntu-server-panel/backend/internal/files"
	"github.com/kantaevsherhan/mini-ubuntu-server-panel/backend/internal/firewall"
	"github.com/kantaevsherhan/mini-ubuntu-server-panel/backend/internal/logs"
	"github.com/kantaevsherhan/mini-ubuntu-server-panel/backend/internal/monitor"
	"github.com/kantaevsherhan/mini-ubuntu-server-panel/backend/internal/processes"
	secretstore "github.com/kantaevsherhan/mini-ubuntu-server-panel/backend/internal/secrets"
	"github.com/kantaevsherhan/mini-ubuntu-server-panel/backend/internal/services"
	"github.com/kantaevsherhan/mini-ubuntu-server-panel/backend/internal/sysinfo"
	"github.com/kantaevsherhan/mini-ubuntu-server-panel/backend/internal/systemusers"
	terminalmanager "github.com/kantaevsherhan/mini-ubuntu-server-panel/backend/internal/terminal"
	"github.com/kantaevsherhan/mini-ubuntu-server-panel/backend/internal/updater"
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
	Logs        logs.Controller
	Files       filemanager.Controller
	Terminal    terminalmanager.Controller
	Tickets     *terminalmanager.TicketStore
	TerminalHub *terminalmanager.Hub
	System      *sysinfo.Collector
	Notifier    monitor.Notifier
	Monitor     *monitor.Monitor
	Updates     updater.Checker
	Secret      string
	Version     string
	DataDir     string
	LogDir      string
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
	if a.Tickets == nil {
		a.Tickets = terminalmanager.NewTicketStore()
	}
	if a.TerminalHub == nil && a.Terminal != nil {
		a.TerminalHub = terminalmanager.NewHub(a.Terminal)
		db := a.DB
		a.TerminalHub.OnExit = func(userID int64, id string) {
			database.Audit(db, userID, "terminal.session.end", "terminal_session", id, `{"commands":"not_recorded"}`, "")
		}
		// Persistent shells outlive browser tabs, so revoked or demoted users are reaped here.
		go a.TerminalHub.Reap(context.Background(), time.Minute, a.terminalUserAllowed)
	}
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
	api.Get("/terminal/ws", a.terminalUpgrade)

	secured := api.Group("", a.authorize)
	secured.Get("/me", func(c *fiber.Ctx) error { return c.JSON(c.Locals("claims")) })
	secured.Post("/auth/password", a.changePassword)
	secured.Post("/auth/logout", a.logout)
	secured.Get("/auth/sessions", a.sessions)
	secured.Delete("/auth/sessions/:id", a.revokeSession)
	secured.Get("/dashboard", a.dashboard)
	secured.Get("/metrics/history", a.metricsHistory)
	secured.Get("/settings/overview", a.requireRole("admin", "operator"), a.settingsOverview)
	secured.Get("/updates", a.requireRole("admin"), a.updateStatus)
	secured.Get("/processes", a.processList)
	secured.Post("/processes/:pid/signal", a.requireRole("admin", "operator"), a.processSignal)
	secured.Get("/services", a.requireRole("admin", "operator"), a.serviceList)
	secured.Post("/services/:unit/action", a.requireRole("admin", "operator"), a.serviceAction)
	secured.Get("/docker/containers", a.requireRole("admin", "operator"), a.dockerContainers)
	secured.Post("/docker/containers", a.requireRole("admin"), a.dockerRunContainer)
	secured.Post("/docker/containers/:id/action", a.requireRole("admin", "operator"), a.dockerContainerAction)
	secured.Get("/docker/containers/:id/logs", a.requireRole("admin", "operator"), a.dockerContainerLogs)
	secured.Get("/docker/images", a.requireRole("admin", "operator"), a.dockerImages)
	secured.Post("/docker/images/pull", a.requireRole("admin"), a.dockerPullImage)
	secured.Get("/docker/images/pulls", a.requireRole("admin", "operator"), a.dockerPullJobs)
	secured.Delete("/docker/images/:id", a.requireRole("admin"), a.dockerRemoveImage)
	secured.Get("/docker/volumes", a.requireRole("admin", "operator"), a.dockerVolumes)
	secured.Post("/docker/volumes", a.requireRole("admin"), a.dockerCreateVolume)
	secured.Delete("/docker/volumes/:name", a.requireRole("admin"), a.dockerRemoveVolume)
	secured.Get("/docker/networks", a.requireRole("admin", "operator"), a.dockerNetworks)
	secured.Delete("/docker/networks/:id", a.requireRole("admin"), a.dockerRemoveNetwork)
	secured.Post("/docker/prune", a.requireRole("admin"), a.dockerPrune)
	secured.Get("/system/info", a.requireRole("admin", "operator"), a.systemInfo)
	secured.Get("/system/ports", a.requireRole("admin", "operator"), a.systemPorts)
	secured.Get("/firewall", a.requireRole("admin", "operator"), a.firewallStatus)
	secured.Post("/firewall/rules", a.requireRole("admin"), a.firewallAddRule)
	secured.Delete("/firewall/rules/:number", a.requireRole("admin"), a.firewallDeleteRule)
	secured.Get("/logs", a.requireRole("admin", "operator"), a.logsList)
	secured.Get("/files/roots", a.requireRole("admin", "operator"), a.fileRoots)
	secured.Get("/files", a.requireRole("admin", "operator"), a.fileList)
	secured.Get("/files/content", a.requireRole("admin", "operator"), a.fileRead)
	secured.Put("/files/content", a.requireRole("admin", "operator"), a.fileWrite)
	secured.Post("/files/directories", a.requireRole("admin", "operator"), a.fileMkdir)
	secured.Post("/files/upload", a.requireRole("admin", "operator"), a.fileUpload)
	secured.Delete("/files", a.requireRole("admin", "operator"), a.fileDelete)
	secured.Post("/terminal/tickets", a.requireRole("admin", "operator"), a.terminalTicket)
	secured.Get("/terminal/sessions", a.requireRole("admin", "operator"), a.terminalSessions)
	secured.Post("/terminal/sessions", a.requireRole("admin", "operator"), a.terminalCreateSession)
	secured.Patch("/terminal/sessions/:id", a.requireRole("admin", "operator"), a.terminalRenameSession)
	secured.Delete("/terminal/sessions/:id", a.requireRole("admin", "operator"), a.terminalCloseSession)
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
	secured.Get("/notifications/monitor", a.requireRole("admin"), a.monitorSettings)
	secured.Put("/notifications/monitor", a.requireRole("admin"), a.updateMonitorSettings)
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
