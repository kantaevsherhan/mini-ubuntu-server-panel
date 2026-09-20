package main

import (
	"context"
	"crypto/rand"
	"embed"
	"encoding/base64"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/compress"
	"github.com/gofiber/fiber/v2/middleware/helmet"
	"github.com/gofiber/fiber/v2/middleware/limiter"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/gofiber/fiber/v2/middleware/requestid"
	"github.com/kantaevsherhan/mini-ubuntu-server-panel/backend/internal/auth"
	"github.com/kantaevsherhan/mini-ubuntu-server-panel/backend/internal/config"
	"github.com/kantaevsherhan/mini-ubuntu-server-panel/backend/internal/database"
	dockermanager "github.com/kantaevsherhan/mini-ubuntu-server-panel/backend/internal/docker"
	filemanager "github.com/kantaevsherhan/mini-ubuntu-server-panel/backend/internal/files"
	"github.com/kantaevsherhan/mini-ubuntu-server-panel/backend/internal/firewall"
	"github.com/kantaevsherhan/mini-ubuntu-server-panel/backend/internal/httpapi"
	"github.com/kantaevsherhan/mini-ubuntu-server-panel/backend/internal/logs"
	"github.com/kantaevsherhan/mini-ubuntu-server-panel/backend/internal/metrics"
	"github.com/kantaevsherhan/mini-ubuntu-server-panel/backend/internal/monitor"
	"github.com/kantaevsherhan/mini-ubuntu-server-panel/backend/internal/notifications"
	"github.com/kantaevsherhan/mini-ubuntu-server-panel/backend/internal/processes"
	"github.com/kantaevsherhan/mini-ubuntu-server-panel/backend/internal/secrets"
	"github.com/kantaevsherhan/mini-ubuntu-server-panel/backend/internal/services"
	"github.com/kantaevsherhan/mini-ubuntu-server-panel/backend/internal/systemusers"
	terminalmanager "github.com/kantaevsherhan/mini-ubuntu-server-panel/backend/internal/terminal"
	"github.com/kantaevsherhan/mini-ubuntu-server-panel/backend/internal/updater"
	"gorm.io/gorm"
)

var version = "dev"

//go:embed web/* web-placeholder.html
var web embed.FS

func main() {
	if handled, err := handleMaintenanceCommand(os.Args[1:]); handled {
		if err != nil {
			fmt.Fprintln(os.Stderr, "maintenance failed:", err)
			os.Exit(1)
		}
		return
	}
	if len(os.Args) == 2 && os.Args[1] == "privileged-user" {
		if err := systemusers.RunPrivileged(os.Stdin); err != nil {
			log.Fatal(err)
		}
		return
	}
	if len(os.Args) == 3 && os.Args[1] == "privileged-secret" && os.Args[2] == "telegram-token" {
		if err := secrets.RunPrivilegedTelegramToken(os.Stdin, secrets.DefaultPath); err != nil {
			log.Fatal(err)
		}
		return
	}
	if len(os.Args) == 2 && os.Args[1] == "privileged-process" {
		if err := processes.RunPrivilegedSignal(os.Stdin); err != nil {
			log.Fatal(err)
		}
		return
	}
	if len(os.Args) == 2 && os.Args[1] == "privileged-service" {
		if err := services.RunPrivilegedAction(os.Stdin); err != nil {
			log.Fatal(err)
		}
		return
	}
	if len(os.Args) == 2 && os.Args[1] == "privileged-firewall" {
		if err := firewall.RunPrivileged(os.Stdin, os.Stdout); err != nil {
			log.Fatal(err)
		}
		return
	}
	if len(os.Args) == 2 && os.Args[1] == "privileged-logs" {
		if err := logs.RunPrivileged(os.Stdin, os.Stdout); err != nil {
			log.Fatal(err)
		}
		return
	}
	if len(os.Args) == 2 && os.Args[1] == "privileged-files" {
		if err := filemanager.RunPrivileged(os.Stdin, os.Stdout, filemanager.DefaultConfigPath); err != nil {
			log.Fatal(err)
		}
		return
	}
	configPath := flag.String("config", "/etc/mini-ubuntu-server/config.yml", "configuration file")
	showVersion := flag.Bool("version", false, "print version")
	flag.Parse()
	if *showVersion {
		fmt.Println(version)
		return
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatal(err)
	}
	db, err := database.Open(filepath.Join(cfg.DataDir, "mini-ubuntu-server.db"))
	if err != nil {
		log.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		log.Fatal(err)
	}
	defer func() { _ = sqlDB.Close() }()
	bootstrap(db)
	systemUserClient, err := systemusers.NewSudoClient()
	if err != nil {
		log.Fatal(err)
	}
	secretWriter, err := secrets.NewSudoWriter()
	if err != nil {
		log.Fatal(err)
	}
	processManager, err := processes.NewManager()
	if err != nil {
		log.Fatal(err)
	}
	serviceManager, err := services.NewManager()
	if err != nil {
		log.Fatal(err)
	}
	dockerManager, err := dockermanager.NewManager()
	if err != nil {
		log.Fatal(err)
	}
	firewallManager, err := firewall.NewManager()
	if err != nil {
		log.Fatal(err)
	}
	logManager, err := logs.NewManager()
	if err != nil {
		log.Fatal(err)
	}
	filesManager, err := filemanager.NewManager(cfg.AllowedDirectories)
	if err != nil {
		log.Fatal(err)
	}
	// The shell is optional: the panel still runs where /bin/bash is absent.
	var terminalController terminalmanager.Controller
	if terminalManager, terminalErr := terminalmanager.NewManager(); terminalErr != nil {
		log.Printf("terminal disabled: %v", terminalErr)
	} else {
		terminalController = terminalManager
	}
	go metrics.NewCollector(db, time.Minute).Start(context.Background())
	notifier := notifications.New(db, notifications.TelegramSender{DB: db})
	go notifier.Run(context.Background())
	serverMonitor := monitor.New(db, notifier, dockerManager, serviceManager)
	go serverMonitor.Run(context.Background())

	app := fiber.New(fiber.Config{
		AppName:               "Mini Ubuntu Server Panel",
		BodyLimit:             3 * 1024 * 1024,
		ReadTimeout:           15 * time.Second,
		WriteTimeout:          30 * time.Second,
		IdleTimeout:           60 * time.Second,
		DisableStartupMessage: true,
		ErrorHandler:          safeErrorHandler,
	})
	app.Use(requestid.New())
	app.Use(recover.New(recover.Config{EnableStackTrace: false}))
	app.Use(helmet.New())
	app.Use(func(c *fiber.Ctx) error {
		c.Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; font-src 'self'; img-src 'self' data:; connect-src 'self' ws: wss:; frame-ancestors 'none'; base-uri 'self'; form-action 'self'")
		c.Set("Permissions-Policy", "camera=(), microphone=(), geolocation=(), payment=(), usb=()")
		c.Set("Cache-Control", "no-store")
		return c.Next()
	})
	app.Use("/api", limiter.New(limiter.Config{
		Max:        300,
		Expiration: time.Minute,
		LimitReached: func(c *fiber.Ctx) error {
			return c.Status(fiber.StatusTooManyRequests).JSON(fiber.Map{"error": "rate_limit_exceeded"})
		},
	}))
	app.Use(compress.New())

	httpapi.API{DB: db, SystemUsers: systemUserClient, Secrets: secretWriter, Processes: processManager, Services: serviceManager, Docker: dockerManager, Firewall: firewallManager, Logs: logManager, Files: filesManager, Terminal: terminalController, Notifier: notifier, Monitor: serverMonitor, Updates: updater.NewHTTPChecker(), Secret: cfg.JWTSecret, Version: version, DataDir: cfg.DataDir, LogDir: cfg.LogDir}.Register(app)
	root, err := fs.Sub(web, "web")
	if err != nil {
		log.Fatal(err)
	}
	app.Use("/", staticFrontend(root))

	log.Printf("Mini Ubuntu Server Panel %s listening on %s", version, cfg.Listen)
	log.Fatal(app.Listen(cfg.Listen))
}

func handleMaintenanceCommand(arguments []string) (bool, error) {
	if len(arguments) == 0 {
		return false, nil
	}
	switch arguments[0] {
	case "update":
		flags := flag.NewFlagSet("update", flag.ContinueOnError)
		flags.SetOutput(os.Stderr)
		requestedVersion := flags.String("version", "latest", "release version such as v1.2.3")
		configPath := flags.String("config", "/etc/mini-ubuntu-server/config.yml", "configuration file")
		if err := flags.Parse(arguments[1:]); err != nil {
			return true, err
		}
		if flags.NArg() > 1 {
			return true, errors.New("update accepts at most one positional version")
		}
		if flags.NArg() == 1 {
			if *requestedVersion != "latest" {
				return true, errors.New("version specified twice")
			}
			*requestedVersion = flags.Arg(0)
		}
		if err := updater.RunUpdate(context.Background(), updater.UpdateOptions{Version: *requestedVersion, CurrentVersion: version, ConfigPath: *configPath}); err != nil {
			return true, err
		}
		fmt.Println("Mini Ubuntu Server Panel update completed successfully.")
		return true, nil
	case "uninstall":
		flags := flag.NewFlagSet("uninstall", flag.ContinueOnError)
		flags.SetOutput(os.Stderr)
		yes := flags.Bool("yes", false, "remove the application without the interactive application prompt")
		removeConfig := flags.Bool("remove-config", false, "remove configuration and secrets")
		removeData := flags.Bool("remove-data", false, "remove SQLite and metric history")
		removeBackups := flags.Bool("remove-backups", false, "remove backups")
		removeUser := flags.Bool("remove-user", false, "remove the system user")
		configPath := flags.String("config", "/etc/mini-ubuntu-server/config.yml", "configuration file")
		if err := flags.Parse(arguments[1:]); err != nil {
			return true, err
		}
		if flags.NArg() != 0 {
			return true, errors.New("uninstall does not accept positional arguments")
		}
		return true, updater.RunUninstall(context.Background(), updater.UninstallOptions{
			Yes: *yes, RemoveConfig: *removeConfig, RemoveData: *removeData,
			RemoveBackups: *removeBackups, RemoveUser: *removeUser, ConfigPath: *configPath,
		})
	default:
		return false, nil
	}
}

func safeErrorHandler(c *fiber.Ctx, err error) error {
	code := fiber.StatusInternalServerError
	errorCode := "internal_error"
	if fiberErr, ok := err.(*fiber.Error); ok {
		code = fiberErr.Code
		switch code {
		case fiber.StatusBadRequest:
			errorCode = "bad_request"
		case fiber.StatusUnauthorized:
			errorCode = "unauthorized"
		case fiber.StatusForbidden:
			errorCode = "forbidden"
		case fiber.StatusNotFound:
			errorCode = "not_found"
		case fiber.StatusRequestEntityTooLarge:
			errorCode = "request_too_large"
		}
	}
	if code >= fiber.StatusInternalServerError {
		log.Printf("request_id=%s status=%d internal request failure", c.GetRespHeader(fiber.HeaderXRequestID), code)
	}
	return c.Status(code).JSON(fiber.Map{"error": errorCode})
}

func staticFrontend(root fs.FS) fiber.Handler {
	return func(c *fiber.Ctx) error {
		path := strings.TrimPrefix(c.Path(), "/")
		if path == "" {
			path = "index.html"
		}
		data, err := fs.ReadFile(root, path)
		if err != nil {
			data, err = fs.ReadFile(root, "index.html")
		}
		if err != nil {
			data, _ = web.ReadFile("web-placeholder.html")
		}
		switch {
		case strings.HasSuffix(path, ".js"):
			c.Type("js")
		case strings.HasSuffix(path, ".css"):
			c.Type("css")
		default:
			c.Type("html")
		}
		return c.Send(data)
	}
}

func bootstrap(db *gorm.DB) {
	var count int64
	if err := db.Model(&database.User{}).Count(&count).Error; err != nil || count > 0 {
		return
	}
	username := os.Getenv("MINI_UBUNTU_SERVER_BOOTSTRAP_USERNAME")
	password := os.Getenv("MINI_UBUNTU_SERVER_BOOTSTRAP_PASSWORD")
	generated := false
	if username == "" {
		username = "admin"
	}
	if password == "" {
		// An empty database would otherwise be unusable; print the temporary password once.
		raw := make([]byte, 18)
		if _, err := rand.Read(raw); err != nil {
			log.Fatal(err)
		}
		password = base64.RawURLEncoding.EncodeToString(raw)
		generated = true
	}
	hash, err := auth.Hash(password)
	if err != nil {
		log.Fatal(err)
	}
	user := database.User{Username: username, DisplayName: "Administrator", PasswordHash: hash, Role: "admin", IsActive: true, MustChangePassword: true}
	if err := db.Create(&user).Error; err != nil {
		log.Fatal(err)
	}
	if generated {
		log.Printf("bootstrap administrator %q created with temporary password: %s (shown once, must be changed at first login)", username, password)
		return
	}
	log.Printf("bootstrap administrator %q created; password is not logged", username)
}
