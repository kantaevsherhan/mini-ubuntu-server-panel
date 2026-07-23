package main

import (
	"context"
	"embed"
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
	"github.com/kantaevsherhan/mini-ubuntu-server-panel/backend/internal/httpapi"
	"github.com/kantaevsherhan/mini-ubuntu-server-panel/backend/internal/metrics"
	"github.com/kantaevsherhan/mini-ubuntu-server-panel/backend/internal/notifications"
	"github.com/kantaevsherhan/mini-ubuntu-server-panel/backend/internal/secrets"
	"github.com/kantaevsherhan/mini-ubuntu-server-panel/backend/internal/systemusers"
	"gorm.io/gorm"
)

var version = "dev"

//go:embed web/* web-placeholder.html
var web embed.FS

func main() {
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
	go metrics.NewCollector(db, time.Minute).Start(context.Background())
	go notifications.New(db, notifications.TelegramSender{DB: db}).Run(context.Background())

	app := fiber.New(fiber.Config{
		AppName:               "Mini Ubuntu Server Panel",
		BodyLimit:             1 * 1024 * 1024,
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

	httpapi.API{DB: db, SystemUsers: systemUserClient, Secrets: secretWriter, Secret: cfg.JWTSecret, Version: version}.Register(app)
	root, err := fs.Sub(web, "web")
	if err != nil {
		log.Fatal(err)
	}
	app.Use("/", staticFrontend(root))

	log.Printf("Mini Ubuntu Server Panel %s listening on %s", version, cfg.Listen)
	log.Fatal(app.Listen(cfg.Listen))
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
	if username == "" || password == "" {
		log.Print("no users exist; set bootstrap environment variables")
		return
	}
	hash, err := auth.Hash(password)
	if err != nil {
		log.Fatal(err)
	}
	user := database.User{Username: username, DisplayName: "Administrator", PasswordHash: hash, Role: "admin", IsActive: true, MustChangePassword: true}
	if err := db.Create(&user).Error; err != nil {
		log.Fatal(err)
	}
	log.Printf("bootstrap administrator %q created; password is not logged", username)
}
