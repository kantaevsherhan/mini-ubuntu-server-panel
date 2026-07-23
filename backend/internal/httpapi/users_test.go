package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/kantaevsherhan/mini-ubuntu-server-panel/backend/internal/auth"
	"github.com/kantaevsherhan/mini-ubuntu-server-panel/backend/internal/database"
	"github.com/kantaevsherhan/mini-ubuntu-server-panel/backend/internal/systemusers"
	"gorm.io/gorm"
)

type fakeSystemUsers struct {
	db             *gorm.DB
	createRequest  *systemusers.CreateRequest
	deleteRequest  *systemusers.DeleteRequest
	injectConflict bool
	deleteError    error
}

func (f *fakeSystemUsers) Exists(string) (bool, error) { return false, nil }

func (f *fakeSystemUsers) Create(_ context.Context, request systemusers.CreateRequest) error {
	f.createRequest = &request
	if f.injectConflict {
		now := time.Now().UTC()
		conflict := database.User{Username: request.Username, DisplayName: "race", PasswordHash: "x", Role: "viewer", IsActive: true, CreatedAt: now, UpdatedAt: now}
		if err := f.db.Create(&conflict).Error; err != nil {
			return err
		}
	}
	return nil
}

func (f *fakeSystemUsers) Delete(_ context.Context, request systemusers.DeleteRequest) error {
	f.deleteRequest = &request
	return f.deleteError
}

func TestDeleteUserRestoresPanelAndSessionsWhenSystemDeleteFails(t *testing.T) {
	db, token := testAuthorizedDB(t)
	systemUsername := "linked-user"
	hash, err := auth.Hash("linked-test-password")
	if err != nil {
		t.Fatal(err)
	}
	target := database.User{Username: "linked", DisplayName: "Linked", PasswordHash: hash, Role: "viewer", IsActive: true, SystemUsername: &systemUsername}
	if err := db.Create(&target).Error; err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	session := database.WebSession{ID: "linked-session", UserID: target.ID, IPAddress: "127.0.0.1", CreatedAt: now, LastSeenAt: now, ExpiresAt: now.Add(time.Hour)}
	if err := db.Create(&session).Error; err != nil {
		t.Fatal(err)
	}
	fake := &fakeSystemUsers{db: db, deleteError: errors.New("user is busy")}
	app := fiber.New()
	API{DB: db, SystemUsers: fake, Secret: "test-secret-that-is-long-enough"}.Register(app)
	payload, _ := json.Marshal(map[string]any{"delete_panel_user": true, "delete_system_user": true, "terminate_sessions": true})
	request := httptest.NewRequest(http.MethodDelete, "/api/v1/users/"+strconv.FormatInt(target.ID, 10), bytes.NewReader(payload))
	request.Header.Set(fiber.HeaderContentType, fiber.MIMEApplicationJSON)
	request.Header.Set(fiber.HeaderAuthorization, "Bearer "+token)
	response, err := app.Test(request)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusConflict {
		t.Fatalf("expected 409, got %d", response.StatusCode)
	}
	var userCount, sessionCount int64
	_ = db.Model(&database.User{}).Where("id = ?", target.ID).Count(&userCount).Error
	_ = db.Model(&database.WebSession{}).Where("id = ?", session.ID).Count(&sessionCount).Error
	if userCount != 1 || sessionCount != 1 {
		t.Fatalf("rollback incomplete: users=%d sessions=%d", userCount, sessionCount)
	}
}

func TestCreatePanelAndSystemUser(t *testing.T) {
	db, token := testAuthorizedDB(t)
	fake := &fakeSystemUsers{db: db}
	app := fiber.New()
	API{DB: db, SystemUsers: fake, Secret: "test-secret-that-is-long-enough"}.Register(app)

	response := performCreateUser(t, app, token, map[string]any{
		"username": "alice", "display_name": "Alice", "password": "long-test-password", "role": "operator",
		"create_panel_user": true, "create_system_user": true, "system_username": "alice-system",
		"home_directory": "/home/alice-system", "shell": "/bin/bash", "create_home": true,
	})
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", response.StatusCode)
	}
	if fake.createRequest == nil || fake.createRequest.Username != "alice-system" {
		t.Fatal("system user create was not called")
	}
	var count int64
	if err := db.Model(&database.User{}).Where("username = ?", "alice").Count(&count).Error; err != nil || count != 1 {
		t.Fatalf("panel user was not persisted: count=%d err=%v", count, err)
	}
}

func TestCreateUserRollsBackSystemUserOnPanelConflict(t *testing.T) {
	db, token := testAuthorizedDB(t)
	fake := &fakeSystemUsers{db: db, injectConflict: true}
	app := fiber.New()
	API{DB: db, SystemUsers: fake, Secret: "test-secret-that-is-long-enough"}.Register(app)

	response := performCreateUser(t, app, token, map[string]any{
		"username": "collision", "display_name": "Collision", "password": "long-test-password", "role": "viewer",
		"create_panel_user": true, "create_system_user": true, "home_directory": "/home/collision",
		"shell": "/bin/bash", "create_home": true,
	})
	if response.StatusCode != http.StatusConflict {
		t.Fatalf("expected 409, got %d", response.StatusCode)
	}
	if fake.deleteRequest == nil || fake.deleteRequest.Username != "collision" || !fake.deleteRequest.DeleteUser || !fake.deleteRequest.RemoveHome {
		t.Fatal("created system user was not rolled back")
	}
}

func testAuthorizedDB(t *testing.T) (*gorm.DB, string) {
	t.Helper()
	db, err := database.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	hash, err := auth.Hash("admin-test-password")
	if err != nil {
		t.Fatal(err)
	}
	admin := database.User{Username: "admin", DisplayName: "Admin", PasswordHash: hash, Role: "admin", IsActive: true}
	if err := db.Create(&admin).Error; err != nil {
		t.Fatal(err)
	}
	token, sessionID, expiresAt, err := auth.Sign("test-secret-that-is-long-enough", admin.ID, admin.Username, admin.Role)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	if err := db.Create(&database.WebSession{ID: sessionID, UserID: admin.ID, IPAddress: "127.0.0.1", CreatedAt: now, LastSeenAt: now, ExpiresAt: expiresAt}).Error; err != nil {
		t.Fatal(err)
	}
	return db, token
}

func performCreateUser(t *testing.T, app *fiber.App, token string, body map[string]any) *http.Response {
	t.Helper()
	payload, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/api/v1/users", bytes.NewReader(payload))
	request.Header.Set(fiber.HeaderContentType, fiber.MIMEApplicationJSON)
	request.Header.Set(fiber.HeaderAuthorization, "Bearer "+token)
	response, err := app.Test(request)
	if err != nil {
		t.Fatal(err)
	}
	return response
}
