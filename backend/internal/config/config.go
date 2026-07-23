package config

import (
	"errors"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Listen   string `yaml:"listen"`
	DataDir  string `yaml:"data_dir"`
	LogDir   string `yaml:"log_dir"`
	JWTSecret string `yaml:"jwt_secret"`
}

func Load(path string) (Config, error) {
	c := Config{Listen: ":8080", DataDir: "/var/lib/mini-ubuntu-server", LogDir: "/var/log/mini-ubuntu-server"}
	if path != "" {
		b, err := os.ReadFile(path)
		if err != nil && !errors.Is(err, os.ErrNotExist) { return c, err }
		if len(b) > 0 { if err := yaml.Unmarshal(b, &c); err != nil { return c, err } }
	}
	if v := os.Getenv("MINI_UBUNTU_SERVER_JWT_SECRET"); v != "" { c.JWTSecret = v }
	if c.JWTSecret == "" { return c, errors.New("MINI_UBUNTU_SERVER_JWT_SECRET is required") }
	if err := os.MkdirAll(c.DataDir, 0750); err != nil { return c, err }
	c.DataDir, _ = filepath.Abs(c.DataDir)
	return c, nil
}
