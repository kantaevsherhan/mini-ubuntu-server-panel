package docker

import (
	"context"
	"sort"
	"strings"

	"github.com/moby/moby/client"
)

// DatabaseTarget is a running database container the database client can
// connect to without the administrator typing its address or password.
// The password itself never leaves the server.
type DatabaseTarget struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Image       string `json:"image"`
	Engine      string `json:"engine"`
	Host        string `json:"host"`
	Port        int    `json:"port"`
	Database    string `json:"database"`
	Username    string `json:"username"`
	AuthSource  string `json:"auth_source"`
	HasPassword bool   `json:"has_password"`
	password    string
}

// Password returns the credential found in the container environment.
func (t DatabaseTarget) Password() string { return t.password }

// DatabaseDiscoverer is implemented by Manager; tests may omit it.
type DatabaseDiscoverer interface {
	DatabaseTargets(context.Context) ([]DatabaseTarget, error)
	DatabaseTarget(context.Context, string) (DatabaseTarget, error)
}

func databaseEngine(image string) string {
	name := strings.ToLower(image)
	if slash := strings.LastIndex(name, "/"); slash >= 0 {
		name = name[slash+1:]
	}
	switch {
	case strings.Contains(name, "postgres"), strings.Contains(name, "postgis"), strings.Contains(name, "timescale"):
		return "postgres"
	case strings.Contains(name, "mysql"), strings.Contains(name, "mariadb"), strings.Contains(name, "percona"):
		return "mysql"
	case strings.HasPrefix(name, "mongo"):
		return "mongodb"
	}
	return ""
}

func envMap(values []string) map[string]string {
	result := map[string]string{}
	for _, item := range values {
		if key, value, ok := strings.Cut(item, "="); ok {
			result[key] = value
		}
	}
	return result
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

// targetFromEnv derives connection defaults from the official images' variables.
func targetFromEnv(engine string, env map[string]string) DatabaseTarget {
	target := DatabaseTarget{Engine: engine}
	switch engine {
	case "postgres":
		target.Port = 5432
		target.Username = firstNonEmpty(env["POSTGRES_USER"], "postgres")
		target.Database = firstNonEmpty(env["POSTGRES_DB"], target.Username)
		target.password = env["POSTGRES_PASSWORD"]
	case "mysql":
		target.Port = 3306
		target.Database = firstNonEmpty(env["MYSQL_DATABASE"], env["MARIADB_DATABASE"], "mysql")
		if user := firstNonEmpty(env["MYSQL_USER"], env["MARIADB_USER"]); user != "" {
			target.Username = user
			target.password = firstNonEmpty(env["MYSQL_PASSWORD"], env["MARIADB_PASSWORD"])
		} else {
			target.Username = "root"
			target.password = firstNonEmpty(env["MYSQL_ROOT_PASSWORD"], env["MARIADB_ROOT_PASSWORD"])
		}
	case "mongodb":
		target.Port = 27017
		target.Username = env["MONGO_INITDB_ROOT_USERNAME"]
		target.password = env["MONGO_INITDB_ROOT_PASSWORD"]
		target.Database = firstNonEmpty(env["MONGO_INITDB_DATABASE"], "admin")
		if target.Username != "" {
			target.AuthSource = "admin"
		}
	}
	target.HasPassword = target.password != ""
	return target
}

func (m Manager) DatabaseTargets(ctx context.Context) ([]DatabaseTarget, error) {
	list, err := m.client.ContainerList(ctx, client.ContainerListOptions{})
	if err != nil {
		return nil, mapError(err)
	}
	targets := []DatabaseTarget{}
	for _, summary := range list.Items {
		if databaseEngine(summary.Image) == "" {
			continue
		}
		target, inspectErr := m.DatabaseTarget(ctx, summary.ID)
		if inspectErr != nil || target.Host == "" {
			continue
		}
		targets = append(targets, target)
	}
	sort.Slice(targets, func(i, j int) bool { return targets[i].Name < targets[j].Name })
	return targets, nil
}

func (m Manager) DatabaseTarget(ctx context.Context, id string) (DatabaseTarget, error) {
	if !containerIDPattern.MatchString(id) {
		return DatabaseTarget{}, ErrInvalid
	}
	inspected, err := m.client.ContainerInspect(ctx, id, client.ContainerInspectOptions{})
	if err != nil {
		return DatabaseTarget{}, mapError(err)
	}
	container := inspected.Container
	if container.Config == nil || container.State == nil || !container.State.Running {
		return DatabaseTarget{}, ErrNotFound
	}
	engine := databaseEngine(container.Config.Image)
	if engine == "" {
		return DatabaseTarget{}, ErrInvalid
	}
	target := targetFromEnv(engine, envMap(container.Config.Env))
	target.ID = container.ID
	target.Name = strings.TrimPrefix(container.Name, "/")
	target.Image = container.Config.Image
	if container.NetworkSettings != nil {
		names := make([]string, 0, len(container.NetworkSettings.Networks))
		for name := range container.NetworkSettings.Networks {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			endpoint := container.NetworkSettings.Networks[name]
			if name == "host" {
				target.Host = "127.0.0.1"
				break
			}
			if endpoint != nil && endpoint.IPAddress.IsValid() {
				target.Host = endpoint.IPAddress.String()
				break
			}
		}
	}
	if target.Host == "" {
		return DatabaseTarget{}, ErrNotFound
	}
	return target, nil
}
