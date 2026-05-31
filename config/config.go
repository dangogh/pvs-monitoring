package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

// Duration wraps time.Duration to support YAML unmarshaling from strings like "30s".
type Duration time.Duration

func (d Duration) Duration() time.Duration { return time.Duration(d) }

func (d *Duration) UnmarshalYAML(value *yaml.Node) error {
	dur, err := time.ParseDuration(value.Value)
	if err != nil {
		return fmt.Errorf("invalid duration %q: %w", value.Value, err)
	}
	*d = Duration(dur)
	return nil
}

const defaultAddr = "ws://192.168.191.155:9002"

// DeviceListConfig holds configuration for the HTTP device-list poller.
type DeviceListConfig struct {
	URL      string   `yaml:"url"`
	// AuthURL overrides the login endpoint (default: URL with https:// + /auth?login).
	AuthURL  string   `yaml:"auth_url,omitempty"`
	Interval Duration `yaml:"interval"`
	Username string   `yaml:"username"`
	// Password is the last 5 characters of the PVS serial number.
	// An empty value disables the device-list poller.
	Password string `yaml:"password"`
}

// Config holds all runtime configuration for pvs-monitor.
type Config struct {
	Addr                     string           `yaml:"addr"`
	ReconnectInitialInterval Duration         `yaml:"reconnect_initial_interval"`
	ReconnectMaxInterval     Duration         `yaml:"reconnect_max_interval"`
	StaleThreshold           Duration         `yaml:"stale_threshold"`
	DeviceList               DeviceListConfig `yaml:"device_list"`
}

// Default returns a Config populated with built-in defaults.
func Default() Config {
	return Config{
		Addr:                     defaultAddr,
		ReconnectInitialInterval: Duration(time.Second),
		ReconnectMaxInterval:     Duration(30 * time.Second),
		StaleThreshold:           Duration(5 * time.Second),
		DeviceList: DeviceListConfig{
			URL:      "http://sunpowerconsole.com",
			Interval: Duration(60 * time.Second),
			Username: "ssm_owner",
		},
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
