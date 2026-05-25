package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

const defaultAddr = "ws://192.168.191.155:9002"

// Config holds all runtime configuration for pvs-monitor.
type Config struct {
	Addr string `yaml:"addr"`
}

// Default returns a Config populated with built-in defaults.
func Default() Config {
	return Config{
		Addr: defaultAddr,
	}
}

// Load reads the config file at path, returning Default() if the file does
// not exist.
func Load(path string) (Config, error) {
	cfg := Default()
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return cfg, nil
	}
	if err != nil {
		return cfg, fmt.Errorf("read config %s: %w", path, err)
	}
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return cfg, fmt.Errorf("parse config %s: %w", path, err)
	}
	return cfg, nil
}

// DefaultPath returns the platform-appropriate default config file path:
// $XDG_CONFIG_HOME/pvs-monitor/config.yaml, falling back to
// ~/.config/pvs-monitor/config.yaml.
func DefaultPath() string {
	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" {
		home, _ := os.UserHomeDir()
		base = filepath.Join(home, ".config")
	}
	return filepath.Join(base, "pvs-monitor", "config.yaml")
}
