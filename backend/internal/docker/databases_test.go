package docker

import "testing"

func TestDatabaseEngine(t *testing.T) {
	for image, want := range map[string]string{
		"postgres:17-alpine": "postgres", "mediagis/nominatim:5.3.2": "", "bitnami/mariadb:11": "mysql",
		"mysql:8.4": "mysql", "mongo:8": "mongodb", "nginx:alpine": "", "timescale/timescaledb:latest-pg16": "postgres",
	} {
		if got := databaseEngine(image); got != want {
			t.Fatalf("%s: got %q", image, got)
		}
	}
}

func TestTargetFromEnv(t *testing.T) {
	pg := targetFromEnv("postgres", envMap([]string{"POSTGRES_USER=app", "POSTGRES_PASSWORD=secret", "PATH=/bin"}))
	if pg.Username != "app" || pg.Database != "app" || pg.Password() != "secret" || !pg.HasPassword || pg.Port != 5432 {
		t.Fatalf("postgres: %+v", pg)
	}
	my := targetFromEnv("mysql", envMap([]string{"MYSQL_ROOT_PASSWORD=root", "MYSQL_DATABASE=shop"}))
	if my.Username != "root" || my.Database != "shop" || my.Password() != "root" {
		t.Fatalf("mysql: %+v", my)
	}
	mongo := targetFromEnv("mongodb", envMap([]string{"MONGO_INITDB_ROOT_USERNAME=admin", "MONGO_INITDB_ROOT_PASSWORD=pw"}))
	if mongo.AuthSource != "admin" || mongo.Database != "admin" || mongo.Password() != "pw" {
		t.Fatalf("mongo: %+v", mongo)
	}
	empty := targetFromEnv("postgres", map[string]string{})
	if empty.Username != "postgres" || empty.HasPassword {
		t.Fatalf("defaults: %+v", empty)
	}
}
