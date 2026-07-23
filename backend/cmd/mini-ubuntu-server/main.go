package main

import (
	"database/sql"
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
)

var version = "dev"

//go:embed web/*
var web embed.FS

func main() {
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
	defer db.Close()
	bootstrap(db)

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

	httpapi.API{DB: db, Secret: cfg.JWTSecret, Version: version}.Register(app)
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
			return fiber.ErrNotFound
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

func bootstrap(db *sql.DB) {
	var count int
	if err := db.QueryRow(`SELECT count(*) FROM users`).Scan(&count); err != nil || count > 0 {
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
	now := time.Now().UTC()
	_, err = db.Exec(`INSERT INTO users(username,display_name,password_hash,role,is_active,must_change_password,created_at,updated_at) VALUES(?,?,?,'admin',1,1,?,?)`, username, "Administrator", hash, now, now)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("bootstrap administrator %q created; password is not logged", username)
}
