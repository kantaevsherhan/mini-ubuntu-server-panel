package config

import (
	"errors"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Listen             string   `yaml:"listen"`
	DataDir            string   `yaml:"data_dir"`
	LogDir             string   `yaml:"log_dir"`
	AllowedDirectories []string `yaml:"allowed_directories"`
	JWTSecret          string   `yaml:"jwt_secret"`
}

func Load(path string) (Config, error) {
	c := Config{Listen: ":8080", DataDir: "/var/lib/mini-ubuntu-server", LogDir: "/var/log/mini-ubuntu-server"}
	if path != "" {
		b, err := os.ReadFile(path)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return c, err
		}
		if len(b) > 0 {
			if err := yaml.Unmarshal(b, &c); err != nil {
				return c, err
			}
		}
	}
	if v := os.Getenv("MINI_UBUNTU_SERVER_JWT_SECRET"); v != "" {
		c.JWTSecret = v
	}
	if len(c.JWTSecret) < 32 {
		return c, errors.New("MINI_UBUNTU_SERVER_JWT_SECRET must contain at least 32 characters")
	}
	if err := os.MkdirAll(c.DataDir, 0750); err != nil {
		return c, err
	}
	c.DataDir, _ = filepath.Abs(c.DataDir)
	c.LogDir, _ = filepath.Abs(c.LogDir)
	if len(c.AllowedDirectories) == 0 {
		c.AllowedDirectories = []string{c.DataDir, c.LogDir}
	}
	allowed, err := NormalizeAllowedDirectories(c.AllowedDirectories)
	if err != nil {
		return c, err
	}
	c.AllowedDirectories = allowed
	return c, nil
}

func LoadAllowedDirectories(path string) ([]string, error) {
	value := struct {
		DataDir            string   `yaml:"data_dir"`
		LogDir             string   `yaml:"log_dir"`
		AllowedDirectories []string `yaml:"allowed_directories"`
	}{DataDir: "/var/lib/mini-ubuntu-server", LogDir: "/var/log/mini-ubuntu-server"}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if err := yaml.Unmarshal(data, &value); err != nil {
		return nil, err
	}
	if len(value.AllowedDirectories) == 0 {
		value.AllowedDirectories = []string{value.DataDir, value.LogDir}
	}
	return NormalizeAllowedDirectories(value.AllowedDirectories)
}

func NormalizeAllowedDirectories(values []string) ([]string, error) {
	result := make([]string, 0, len(values))
	seen := make(map[string]bool)
	for _, value := range values {
		value = filepath.Clean(value)
		if !filepath.IsAbs(value) || value == string(filepath.Separator) {
			return nil, errors.New("allowed directories must be absolute and cannot include filesystem root")
		}
		if seen[value] {
			continue
		}
		seen[value] = true
		result = append(result, value)
	}
	if len(result) == 0 || len(result) > 32 {
		return nil, errors.New("between 1 and 32 allowed directories are required")
	}
	return result, nil
}
