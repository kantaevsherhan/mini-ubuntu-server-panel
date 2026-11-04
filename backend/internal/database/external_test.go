package database

import (
	"context"
	"os"
	"testing"
)

func TestExternalValidation(t *testing.T) {
	good := ExternalRequest{Engine: "postgres", Host: "127.0.0.1", Port: 5432, Database: "app"}
	if err := ValidateExternal(good); err != nil {
		t.Fatal(err)
	}
	bad := []ExternalRequest{good, good, good, good, good, good, good}
	bad[0].Engine = "sqlite"
	bad[1].Host = "localhost/path"
	bad[2].Database = "app; DROP TABLE x"
	bad[3].Table = "foo` WHERE 1=1"
	bad[4].Offset = -1
	bad[5].Port = 65536
	bad[6].AuthSource = "admin?tls=false"
	for _, r := range bad {
		if ValidateExternal(r) == nil {
			t.Fatal("unsafe request accepted")
		}
	}
}

// Optional integration checks run against isolated disposable database containers.
func TestExternalIntegration(t *testing.T) {
	engine := os.Getenv("MUSP_TEST_DATABASE_ENGINE")
	if engine == "" {
		t.Skip("isolated database not configured")
	}
	port := 5432
	if engine == "mysql" {
		port = 3306
	}
	if engine == "mongodb" {
		port = 27017
	}
	r := ExternalRequest{Engine: engine, Host: "musp-test-db", Port: port, Database: "fixture", Username: "tester", Password: "fixture-password", AuthSource: "admin"}
	result, err := BrowseExternal(context.Background(), r)
	if err != nil {
		t.Fatalf("list failed: %v", err)
	}
	if len(result.Tables) != 1 || result.Tables[0] != "records" {
		t.Fatalf("unexpected metadata: %+v", result.Tables)
	}
	r.Table = "records"
	result, err = BrowseExternal(context.Background(), r)
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}
	if len(result.Rows) != 100 || !result.HasMore {
		t.Fatalf("pagination incorrect: %d %v", len(result.Rows), result.HasMore)
	}
	r.Offset = 100
	result, err = BrowseExternal(context.Background(), r)
	if err != nil || len(result.Rows) != 1 || result.HasMore {
		t.Fatalf("second page incorrect: %d %v", len(result.Rows), err)
	}
	r.Password = "wrong-password"
	if _, err = BrowseExternal(context.Background(), r); err == nil {
		t.Fatal("invalid credentials accepted")
	}
}
