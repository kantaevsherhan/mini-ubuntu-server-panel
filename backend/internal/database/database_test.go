package database

import (
	"path/filepath"
	"testing"
)

func TestMigrationsAreAppliedOnce(t *testing.T) {
	path := filepath.Join(t.TempDir(), "panel.db")
	for attempt := 0; attempt < 2; attempt++ {
		db, err := Open(path)
		if err != nil {
			t.Fatal(err)
		}
		var count int
		if err := db.QueryRow(`SELECT count(*) FROM schema_migrations`).Scan(&count); err != nil {
			db.Close()
			t.Fatal(err)
		}
		if count != 3 {
			db.Close()
			t.Fatalf("expected 3 applied migrations, got %d", count)
		}
		if err := db.Close(); err != nil {
			t.Fatal(err)
		}
	}
}

func TestForeignKeysAreEnabled(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "panel.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var enabled int
	if err := db.QueryRow(`PRAGMA foreign_keys`).Scan(&enabled); err != nil {
		t.Fatal(err)
	}
	if enabled != 1 {
		t.Fatal("foreign keys are disabled")
	}
}
