package httpapi

import (
	"bytes"
	"context"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/kantaevsherhan/mini-ubuntu-server-panel/backend/internal/database"
	filemanager "github.com/kantaevsherhan/mini-ubuntu-server-panel/backend/internal/files"
)

type fakeFiles struct {
	writtenPath string
	written     []byte
	deleted     string
}

func (f *fakeFiles) Roots() []filemanager.Root {
	return []filemanager.Root{{ID: 0, Name: "data", Path: "/var/lib/data"}}
}
func (f *fakeFiles) List(context.Context, int, string) ([]filemanager.Entry, error) {
	return []filemanager.Entry{{Name: "app.yml", Path: "app.yml", ModifiedAt: time.Unix(1, 0).UTC()}}, nil
}
func (f *fakeFiles) Read(_ context.Context, _ int, path string) (filemanager.File, error) {
	return filemanager.File{Path: path, Content: "enabled: true\n"}, nil
}
func (f *fakeFiles) Write(_ context.Context, _ int, path string, content []byte) error {
	f.writtenPath, f.written = path, content
	return nil
}
func (f *fakeFiles) Mkdir(context.Context, int, string) error { return nil }
func (f *fakeFiles) Delete(_ context.Context, _ int, path string) error {
	f.deleted = path
	return nil
}

func TestFileRoutesWriteDeleteAndAudit(t *testing.T) {
	db, token := testAuthorizedDB(t)
	controller := &fakeFiles{}
	app := fiber.New()
	API{DB: db, Files: controller, Secret: "test-secret-that-is-long-enough"}.Register(app)
	writeRequest := httptest.NewRequest(http.MethodPut, "/api/v1/files/content", bytes.NewBufferString(`{"root":0,"path":"app.yml","content":"enabled: true\n"}`))
	writeRequest.Header.Set(fiber.HeaderAuthorization, "Bearer "+token)
	writeRequest.Header.Set(fiber.HeaderContentType, fiber.MIMEApplicationJSON)
	writeResponse, err := app.Test(writeRequest)
	if err != nil || writeResponse.StatusCode != http.StatusNoContent || controller.writtenPath != "app.yml" {
		t.Fatalf("write failed: status=%d path=%q err=%v", writeResponse.StatusCode, controller.writtenPath, err)
	}
	deleteRequest := httptest.NewRequest(http.MethodDelete, "/api/v1/files?root=0&path=app.yml", nil)
	deleteRequest.Header.Set(fiber.HeaderAuthorization, "Bearer "+token)
	deleteResponse, err := app.Test(deleteRequest)
	if err != nil || deleteResponse.StatusCode != http.StatusNoContent || controller.deleted != "app.yml" {
		t.Fatalf("delete failed: status=%d path=%q err=%v", deleteResponse.StatusCode, controller.deleted, err)
	}
	var count int64
	if err := db.Model(&database.AuditEvent{}).Where("action IN ?", []string{"file.write", "file.delete"}).Count(&count).Error; err != nil || count != 2 {
		t.Fatalf("audit records missing: count=%d err=%v", count, err)
	}
}

func TestFileUploadUsesSafeRelativeName(t *testing.T) {
	db, token := testAuthorizedDB(t)
	controller := &fakeFiles{}
	app := fiber.New(fiber.Config{BodyLimit: 3 * 1024 * 1024})
	API{DB: db, Files: controller, Secret: "test-secret-that-is-long-enough"}.Register(app)
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	_ = writer.WriteField("root", "0")
	_ = writer.WriteField("path", "configs")
	part, err := writer.CreateFormFile("file", "app.yml")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = part.Write([]byte("enabled: true\n"))
	_ = writer.Close()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/files/upload", &body)
	request.Header.Set(fiber.HeaderAuthorization, "Bearer "+token)
	request.Header.Set(fiber.HeaderContentType, writer.FormDataContentType())
	response, err := app.Test(request)
	if err != nil || response.StatusCode != http.StatusNoContent || controller.writtenPath != "configs/app.yml" {
		t.Fatalf("upload failed: status=%d path=%q err=%v", response.StatusCode, controller.writtenPath, err)
	}
}

func TestFileRoutesRejectTraversal(t *testing.T) {
	db, token := testAuthorizedDB(t)
	controller := &fakeFiles{}
	app := fiber.New()
	API{DB: db, Files: controller, Secret: "test-secret-that-is-long-enough"}.Register(app)
	request := httptest.NewRequest(http.MethodGet, "/api/v1/files/content?root=0&path=..%2Fetc%2Fpasswd", nil)
	request.Header.Set(fiber.HeaderAuthorization, "Bearer "+token)
	response, err := app.Test(request)
	if err != nil || response.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("traversal was not rejected: status=%d err=%v", response.StatusCode, err)
	}
}
