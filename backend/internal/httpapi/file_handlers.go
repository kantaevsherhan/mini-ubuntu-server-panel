package httpapi

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/kantaevsherhan/mini-ubuntu-server-panel/backend/internal/auth"
	"github.com/kantaevsherhan/mini-ubuntu-server-panel/backend/internal/database"
	filemanager "github.com/kantaevsherhan/mini-ubuntu-server-panel/backend/internal/files"
)

func (a API) fileRoots(c *fiber.Ctx) error {
	if a.Files == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"error": "files_service_unavailable"})
	}
	return c.JSON(a.Files.Roots())
}

func (a API) fileList(c *fiber.Ctx) error {
	root, relative, err := parseFileLocation(c.Query("root"), c.Query("path"), true)
	if err != nil {
		return c.Status(fiber.StatusUnprocessableEntity).JSON(fiber.Map{"error": "file_location_invalid"})
	}
	if a.Files == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"error": "files_service_unavailable"})
	}
	ctx, cancel := context.WithTimeout(c.UserContext(), 15*time.Second)
	defer cancel()
	entries, err := a.Files.List(ctx, root, relative)
	if err != nil {
		return c.Status(fiber.StatusConflict).JSON(fiber.Map{"error": "file_list_failed"})
	}
	return c.JSON(entries)
}

func (a API) fileRead(c *fiber.Ctx) error {
	root, relative, err := parseFileLocation(c.Query("root"), c.Query("path"), false)
	if err != nil {
		return c.Status(fiber.StatusUnprocessableEntity).JSON(fiber.Map{"error": "file_location_invalid"})
	}
	if a.Files == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"error": "files_service_unavailable"})
	}
	ctx, cancel := context.WithTimeout(c.UserContext(), 15*time.Second)
	defer cancel()
	file, err := a.Files.Read(ctx, root, relative)
	if err != nil {
		return c.Status(fiber.StatusUnprocessableEntity).JSON(fiber.Map{"error": "file_read_failed"})
	}
	return c.JSON(file)
}

func (a API) fileWrite(c *fiber.Ctx) error {
	var request struct {
		Root    int    `json:"root"`
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	if err := c.BodyParser(&request); err != nil {
		return fiber.ErrBadRequest
	}
	if _, _, err := parseFileLocation(strconv.Itoa(request.Root), request.Path, false); err != nil || len(request.Content) > filemanager.MaxContentBytes {
		return c.Status(fiber.StatusUnprocessableEntity).JSON(fiber.Map{"error": "file_write_invalid"})
	}
	if a.Files == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"error": "files_service_unavailable"})
	}
	ctx, cancel := context.WithTimeout(c.UserContext(), 20*time.Second)
	defer cancel()
	if err := a.Files.Write(ctx, request.Root, request.Path, []byte(request.Content)); err != nil {
		return c.Status(fiber.StatusConflict).JSON(fiber.Map{"error": "file_write_failed"})
	}
	a.auditFileMutation(c, "file.write", request.Root, request.Path, fmt.Sprintf(`{"bytes":%d}`, len(request.Content)))
	return c.SendStatus(fiber.StatusNoContent)
}

func (a API) fileMkdir(c *fiber.Ctx) error {
	var request struct {
		Root int    `json:"root"`
		Path string `json:"path"`
	}
	if err := c.BodyParser(&request); err != nil {
		return fiber.ErrBadRequest
	}
	if _, _, err := parseFileLocation(strconv.Itoa(request.Root), request.Path, false); err != nil {
		return c.Status(fiber.StatusUnprocessableEntity).JSON(fiber.Map{"error": "directory_create_invalid"})
	}
	if a.Files == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"error": "files_service_unavailable"})
	}
	ctx, cancel := context.WithTimeout(c.UserContext(), 15*time.Second)
	defer cancel()
	if err := a.Files.Mkdir(ctx, request.Root, request.Path); err != nil {
		return c.Status(fiber.StatusConflict).JSON(fiber.Map{"error": "directory_create_failed"})
	}
	a.auditFileMutation(c, "file.mkdir", request.Root, request.Path, `{}`)
	return c.SendStatus(fiber.StatusNoContent)
}

func (a API) fileUpload(c *fiber.Ctx) error {
	root, relative, err := parseFileLocation(c.FormValue("root"), c.FormValue("path"), true)
	if err != nil {
		return c.Status(fiber.StatusUnprocessableEntity).JSON(fiber.Map{"error": "file_upload_invalid"})
	}
	header, err := c.FormFile("file")
	if err != nil || header.Size < 0 || header.Size > filemanager.MaxContentBytes || header.Filename == "" || header.Filename == "." || header.Filename == ".." || strings.ContainsAny(header.Filename, `/\`) {
		return c.Status(fiber.StatusUnprocessableEntity).JSON(fiber.Map{"error": "file_upload_invalid"})
	}
	handle, err := header.Open()
	if err != nil {
		return c.Status(fiber.StatusUnprocessableEntity).JSON(fiber.Map{"error": "file_upload_invalid"})
	}
	defer func() { _ = handle.Close() }()
	data, err := io.ReadAll(io.LimitReader(handle, filemanager.MaxContentBytes+1))
	if err != nil || len(data) > filemanager.MaxContentBytes {
		return c.Status(fiber.StatusUnprocessableEntity).JSON(fiber.Map{"error": "file_upload_invalid"})
	}
	target := path.Join(relative, header.Filename)
	if a.Files == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"error": "files_service_unavailable"})
	}
	ctx, cancel := context.WithTimeout(c.UserContext(), 20*time.Second)
	defer cancel()
	if err := a.Files.Write(ctx, root, target, data); err != nil {
		return c.Status(fiber.StatusConflict).JSON(fiber.Map{"error": "file_upload_failed"})
	}
	a.auditFileMutation(c, "file.upload", root, target, fmt.Sprintf(`{"bytes":%d}`, len(data)))
	return c.SendStatus(fiber.StatusNoContent)
}

func (a API) fileDelete(c *fiber.Ctx) error {
	root, relative, err := parseFileLocation(c.Query("root"), c.Query("path"), false)
	if err != nil {
		return c.Status(fiber.StatusUnprocessableEntity).JSON(fiber.Map{"error": "file_location_invalid"})
	}
	if a.Files == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"error": "files_service_unavailable"})
	}
	ctx, cancel := context.WithTimeout(c.UserContext(), 15*time.Second)
	defer cancel()
	if err := a.Files.Delete(ctx, root, relative); err != nil {
		return c.Status(fiber.StatusConflict).JSON(fiber.Map{"error": "file_delete_failed"})
	}
	a.auditFileMutation(c, "file.delete", root, relative, `{}`)
	return c.SendStatus(fiber.StatusNoContent)
}

func parseFileLocation(rootValue, relative string, allowRoot bool) (int, string, error) {
	root, err := strconv.Atoi(rootValue)
	if err != nil || root < 0 || root > 31 || len(relative) > 4096 || strings.ContainsRune(relative, 0) || strings.HasPrefix(relative, "/") {
		return 0, "", errors.New("invalid file location")
	}
	relative = path.Clean(relative)
	if relative == "." {
		relative = ""
	}
	if (!allowRoot && relative == "") || relative == ".." || strings.HasPrefix(relative, "../") {
		return 0, "", errors.New("invalid file location")
	}
	return root, relative, nil
}

func (a API) auditFileMutation(c *fiber.Ctx, action string, root int, relative, details string) {
	claims := c.Locals("claims").(*auth.Claims)
	database.Audit(a.DB, claims.UserID, action, "file", fmt.Sprintf("%d:%s", root, relative), details, c.IP())
}
