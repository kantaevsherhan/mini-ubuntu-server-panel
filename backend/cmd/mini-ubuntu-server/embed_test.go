package main

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"testing/fstest"

	"github.com/gofiber/fiber/v2"
)

// TestEmbeddedFrontendIsComplete guards against go:embed silently dropping
// built assets (for example files prefixed with "_" or ".").
func TestEmbeddedFrontendIsComplete(t *testing.T) {
	err := fs.WalkDir(os.DirFS("web"), ".", func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		if _, err := fs.Stat(web, "web/"+path); err != nil {
			t.Errorf("built file web/%s is not embedded: %v", path, err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestStaticFrontendMissingAssetIsNotFound(t *testing.T) {
	app := fiber.New()
	app.Use("/", staticFrontend(fstest.MapFS{"index.html": {Data: []byte("<!doctype html>")}}))
	for path, want := range map[string]int{"/assets/missing.js": fiber.StatusNotFound, "/docker": fiber.StatusOK} {
		response, err := app.Test(httptest.NewRequest(http.MethodGet, path, nil))
		if err != nil {
			t.Fatal(err)
		}
		if response.StatusCode != want {
			t.Errorf("%s: status %d, want %d", path, response.StatusCode, want)
		}
	}
}
