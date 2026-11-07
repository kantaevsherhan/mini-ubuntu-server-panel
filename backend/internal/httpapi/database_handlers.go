package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/kantaevsherhan/mini-ubuntu-server-panel/backend/internal/database"
	dockermanager "github.com/kantaevsherhan/mini-ubuntu-server-panel/backend/internal/docker"
)

var databaseSlots = make(chan struct{}, 4)

func (a API) databaseBrowse(c *fiber.Ctx) error {
	c.Set("Cache-Control", "no-store")
	var request database.ExternalRequest
	if len(c.Body()) > 16384 || c.BodyParser(&request) != nil {
		return fiber.ErrBadRequest
	}
	if err := database.ValidateExternal(request); err != nil {
		return c.Status(422).JSON(fiber.Map{"error": "database_request_invalid"})
	}
	select {
	case databaseSlots <- struct{}{}:
		defer func() { <-databaseSlots }()
	default:
		return c.Status(429).JSON(fiber.Map{"error": "database_busy"})
	}
	ctx, cancel := context.WithTimeout(c.UserContext(), 12*time.Second)
	defer cancel()
	result, err := database.BrowseExternal(ctx, request)
	// Record only engine and outcome, never connection credentials or returned data.
	outcome := "success"
	if err != nil {
		outcome = "failed"
	}
	a.dockerAudit(c, "database.browse", "external_database", request.Engine, `{"outcome":"`+outcome+`"}`)
	if errors.Is(err, database.ErrExternalInvalid) {
		return c.Status(422).JSON(fiber.Map{"error": "database_request_invalid"})
	}
	if errors.Is(err, database.ErrExternalTooLarge) {
		return c.Status(413).JSON(fiber.Map{"error": "database_result_too_large"})
	}
	if err != nil {
		return c.Status(502).JSON(fiber.Map{"error": "database_connection_failed"})
	}
	return c.JSON(result)
}

// databaseClientRequestLimit bounds SQL scripts and edited documents.
const databaseClientRequestLimit = 2 << 20

func (a API) databaseContainers(c *fiber.Ctx) error {
	c.Set("Cache-Control", "no-store")
	discoverer, ok := a.Docker.(dockermanager.DatabaseDiscoverer)
	if a.Docker == nil || !ok {
		return c.JSON([]dockermanager.DatabaseTarget{})
	}
	ctx, cancel := context.WithTimeout(c.UserContext(), 10*time.Second)
	defer cancel()
	targets, err := discoverer.DatabaseTargets(ctx)
	if err != nil {
		return a.dockerUnavailable(c)
	}
	return c.JSON(targets)
}

// resolveDatabaseContainer fills address and credentials from a running
// database container; explicitly entered values take precedence.
func (a API) resolveDatabaseContainer(ctx context.Context, request *database.ClientRequest) error {
	if request.Container == "" {
		return nil
	}
	discoverer, ok := a.Docker.(dockermanager.DatabaseDiscoverer)
	if a.Docker == nil || !ok {
		return dockermanager.ErrNotFound
	}
	target, err := discoverer.DatabaseTarget(ctx, request.Container)
	if err != nil {
		return err
	}
	request.Engine = target.Engine
	request.Host = target.Host
	if request.Port == 0 {
		request.Port = target.Port
	}
	if request.Database == "" {
		request.Database = target.Database
	}
	if request.Username == "" {
		request.Username = target.Username
		if request.Password == "" {
			request.Password = target.Password()
		}
	}
	if request.AuthSource == "" {
		request.AuthSource = target.AuthSource
	}
	return nil
}

func databaseAuditDetails(op string, request database.ClientRequest, outcome string) string {
	details := map[string]any{"outcome": outcome}
	if request.Table != "" {
		details["table"] = request.Table
	}
	if op == "rows" {
		details["action"] = request.Action
		details["rows"] = max(len(request.Keys), 1)
	}
	if op == "query" {
		// Only statement kinds are recorded: scripts may contain secrets.
		kinds := []string{}
		for _, statement := range database.SplitStatements(request.SQL, request.Engine) {
			word := strings.ToUpper(strings.Fields(statement + " x")[0])
			if len(word) > 16 {
				word = word[:16]
			}
			kinds = append(kinds, word)
			if len(kinds) == 20 {
				break
			}
		}
		details["statements"] = kinds
	}
	encoded, _ := json.Marshal(details)
	return string(encoded)
}

func (a API) databaseClient(op string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		c.Set("Cache-Control", "no-store")
		var request database.ClientRequest
		if len(c.Body()) > databaseClientRequestLimit || c.BodyParser(&request) != nil {
			return fiber.ErrBadRequest
		}
		resolveCtx, resolveCancel := context.WithTimeout(c.UserContext(), 5*time.Second)
		err := a.resolveDatabaseContainer(resolveCtx, &request)
		resolveCancel()
		if err != nil {
			if errors.Is(err, dockermanager.ErrInvalid) {
				return c.Status(422).JSON(fiber.Map{"error": "database_request_invalid"})
			}
			return c.Status(404).JSON(fiber.Map{"error": "database_container_not_found"})
		}
		if err = database.ValidateClient(request); err != nil {
			return c.Status(422).JSON(fiber.Map{"error": "database_request_invalid"})
		}
		select {
		case databaseSlots <- struct{}{}:
			defer func() { <-databaseSlots }()
		default:
			return c.Status(429).JSON(fiber.Map{"error": "database_busy"})
		}
		timeout := 15 * time.Second
		if op == "query" {
			timeout = 32 * time.Second
		}
		ctx, cancel := context.WithTimeout(c.UserContext(), timeout)
		defer cancel()
		result, err := database.RunClient(ctx, op, request)
		outcome := "success"
		if err != nil {
			outcome = "failed"
		}
		if op == "query" || op == "rows" {
			a.dockerAudit(c, "database."+op, "external_database", request.Engine, databaseAuditDetails(op, request, outcome))
		}
		var statementErr database.StatementError
		switch {
		case err == nil:
			return c.JSON(result)
		case errors.As(err, &statementErr):
			return c.Status(400).JSON(fiber.Map{"error": "database_statement_failed", "message": statementErr.Message})
		case errors.Is(err, database.ErrExternalInvalid):
			return c.Status(422).JSON(fiber.Map{"error": "database_request_invalid"})
		case errors.Is(err, database.ErrNoPrimaryKey):
			return c.Status(409).JSON(fiber.Map{"error": "database_no_primary_key"})
		case errors.Is(err, database.ErrUnsupported):
			return c.Status(422).JSON(fiber.Map{"error": "database_unsupported"})
		case errors.Is(err, database.ErrExternalTooLarge):
			return c.Status(413).JSON(fiber.Map{"error": "database_result_too_large"})
		default:
			return c.Status(502).JSON(fiber.Map{"error": "database_connection_failed"})
		}
	}
}
