package config

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
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

// IsUnconfigured reports whether the config still contains the placeholder
// address shipped in the example config file.
func (c *Config) IsUnconfigured() bool {
	return strings.Contains(c.Addr, "YOUR-PVS6-IP")
}

// deviceListURLFromAddr derives the device list HTTP URL from the WebSocket address.
// For example: "ws://192.168.191.155:9002" → "http://192.168.191.155"
func deviceListURLFromAddr(wsAddr string) string {
	u, err := url.Parse(wsAddr)
	if err != nil {
		return "http://192.168.191.155" // fallback to default
	}
	return fmt.Sprintf("http://%s", u.Hostname())
}

// DeviceListConfig holds configuration for the HTTP device-list poller.
type DeviceListConfig struct {
	URL string `yaml:"url"`
	// AuthURL overrides the login endpoint (default: URL with https:// + /auth?login).
	AuthURL  string   `yaml:"auth_url,omitempty"`
	Interval Duration `yaml:"interval"`
	Username string   `yaml:"username"`
	// Password is the last 5 characters of the PVS serial number.
	// An empty value disables the device-list poller.
	Password string `yaml:"password"`
	// TLSFingerprint is the expected SHA-256 fingerprint of the PVS6 TLS certificate
	// (hex, with or without colon separators). When set, the certificate is pinned
	// instead of blindly skipping verification. Leave empty to use legacy InsecureSkipVerify.
	TLSFingerprint string `yaml:"tls_fingerprint,omitempty"`
}

// Config holds all runtime configuration for pvs-monitor.
type Config struct {
	Addr                     string   `yaml:"addr"`
	ReconnectInitialInterval Duration `yaml:"reconnect_initial_interval"`
	ReconnectMaxInterval     Duration `yaml:"reconnect_max_interval"`
	StaleThreshold           Duration `yaml:"stale_threshold"`
	// ReadTimeout bounds how long a single WebSocket read may block before the
	// connection is treated as dead. The PVS6 sends power frames about once a
	// second, so silence this long is a fault, not jitter. Without it the read
	// blocks until TCP gives up, which took 58 hours on 2026-09-22. Kept
	// separate from StaleThreshold, which happens to share the default but
	// answers a different question (how old a reading may be before API
	// callers should distrust it).
	ReadTimeout Duration         `yaml:"read_timeout"`
	DeviceList  DeviceListConfig `yaml:"device_list"`
	// Timezone is the IANA zone name (e.g. "America/Los_Angeles") of the site
	// where the PVS6 is installed. pvs-ui uses it to bucket and label charts
	// by the site's calendar day regardless of the viewing browser's own
	// timezone (see #96). Defaults to the detected system timezone; set this
	// explicitly if pvs-monitor doesn't run on the same site as the PVS6.
	Timezone string `yaml:"timezone,omitempty"`
}

// Default returns a Config populated with built-in defaults.
func Default() Config {
	return Config{
		Addr:                     defaultAddr,
		ReconnectInitialInterval: Duration(time.Second),
		ReconnectMaxInterval:     Duration(30 * time.Second),
		// Above ReconnectMaxInterval: a reconnect can take that long, and a
		// shorter threshold reports stale data during normal recovery.
		StaleThreshold: Duration(60 * time.Second),
		ReadTimeout:    Duration(60 * time.Second),
		DeviceList: DeviceListConfig{
			URL:      deviceListURLFromAddr(defaultAddr),
			Interval: Duration(60 * time.Second),
			Username: "ssm_owner",
		},
		Timezone: systemTimezone(),
	}
}

// systemTimezone best-effort detects the IANA zone name of the host running
// this process, so a fresh install reports the site's actual timezone
// without needing manual configuration. Returns "" if detection fails.
func systemTimezone() string {
	if tz := os.Getenv("TZ"); tz != "" {
		return tz
	}
	// /etc/localtime is conventionally a symlink into the zoneinfo database
	// on Linux and macOS; its target's tail is the IANA zone name.
	if target, err := os.Readlink("/etc/localtime"); err == nil {
		const marker = "zoneinfo/"
		if i := strings.Index(target, marker); i >= 0 {
			return target[i+len(marker):]
		}
	}
	return ""
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
