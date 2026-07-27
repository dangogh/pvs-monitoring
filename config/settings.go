package config

import (
	"context"
	"fmt"
	"time"
)

// Setting keys for the DB-backed settings table. These are the domain/
// operational config items that can be edited at runtime (via the admin UI).
// Bootstrap/launch parameters (DB path, listen addresses, TLS paths) stay in
// the file and are intentionally absent here.
const (
	KeyAddr                     = "addr"
	KeyReconnectInitialInterval = "reconnect_initial_interval"
	KeyReconnectMaxInterval     = "reconnect_max_interval"
	KeyStaleThreshold           = "stale_threshold"
	KeyDeviceListURL            = "device_list.url"
	KeyDeviceListAuthURL        = "device_list.auth_url"
	KeyDeviceListInterval       = "device_list.interval"
	KeyDeviceListUsername       = "device_list.username"
	KeyDeviceListPassword       = "device_list.password"
	KeyDeviceListTLSFingerprint = "device_list.tls_fingerprint"
)

// SettingsReader reads persisted settings. Defined here (rather than importing
// pvs.Store) to avoid an import cycle: pvs already imports config.
type SettingsReader interface {
	Settings(ctx context.Context) (map[string]string, error)
}

// SettingsWriter persists individual settings.
type SettingsWriter interface {
	SetSetting(ctx context.Context, key, value string) error
}

// SettingsStore both reads and writes settings.
type SettingsStore interface {
	SettingsReader
	SettingsWriter
}

// LoadWithStore loads the file config (defaults + config.yaml) and then
// overlays any values persisted in the DB settings table on top. The resulting
// precedence is DB > file > default. Callers still apply flag/env overrides
// afterwards, keeping the full order: flag > env > DB > file > default.
func LoadWithStore(ctx context.Context, path string, store SettingsReader) (Config, error) {
	cfg, err := Load(path)
	if err != nil {
		return cfg, err
	}
	settings, err := store.Settings(ctx)
	if err != nil {
		return cfg, fmt.Errorf("load settings: %w", err)
	}
	if err := applySettings(&cfg, settings); err != nil {
		return cfg, err
	}
	return cfg, nil
}

// applySettings overlays the given key→value settings onto cfg. Unknown keys
// are ignored; missing keys leave the existing value untouched.
func applySettings(cfg *Config, m map[string]string) error {
	for key, val := range m {
		switch key {
		case KeyAddr:
			cfg.Addr = val
		case KeyReconnectInitialInterval:
			if err := parseDurationSetting(key, val, &cfg.ReconnectInitialInterval); err != nil {
				return err
			}
		case KeyReconnectMaxInterval:
			if err := parseDurationSetting(key, val, &cfg.ReconnectMaxInterval); err != nil {
				return err
			}
		case KeyStaleThreshold:
			if err := parseDurationSetting(key, val, &cfg.StaleThreshold); err != nil {
				return err
			}
		case KeyDeviceListURL:
			cfg.DeviceList.URL = val
		case KeyDeviceListAuthURL:
			cfg.DeviceList.AuthURL = val
		case KeyDeviceListInterval:
			if err := parseDurationSetting(key, val, &cfg.DeviceList.Interval); err != nil {
				return err
			}
		case KeyDeviceListUsername:
			cfg.DeviceList.Username = val
		case KeyDeviceListPassword:
			cfg.DeviceList.Password = val
		case KeyDeviceListTLSFingerprint:
			cfg.DeviceList.TLSFingerprint = val
		}
	}
	return nil
}

func parseDurationSetting(key, val string, dst *Duration) error {
	d, err := time.ParseDuration(val)
	if err != nil {
		return fmt.Errorf("invalid duration for %q: %w", key, err)
	}
	*dst = Duration(d)
	return nil
}

// SettingsFromConfig serializes the DB-backed fields of cfg into a key→value
// map, the inverse of applySettings. Used to seed the settings table and to
// surface effective config to the admin UI.
func SettingsFromConfig(cfg Config) map[string]string {
	return map[string]string{
		KeyAddr:                     cfg.Addr,
		KeyReconnectInitialInterval: cfg.ReconnectInitialInterval.Duration().String(),
		KeyReconnectMaxInterval:     cfg.ReconnectMaxInterval.Duration().String(),
		KeyStaleThreshold:           cfg.StaleThreshold.Duration().String(),
		KeyDeviceListURL:            cfg.DeviceList.URL,
		KeyDeviceListAuthURL:        cfg.DeviceList.AuthURL,
		KeyDeviceListInterval:       cfg.DeviceList.Interval.Duration().String(),
		KeyDeviceListUsername:       cfg.DeviceList.Username,
		KeyDeviceListPassword:       cfg.DeviceList.Password,
		KeyDeviceListTLSFingerprint: cfg.DeviceList.TLSFingerprint,
	}
}

// SeedSettingsIfEmpty populates the settings table from cfg when it is empty,
// so existing file-based installs migrate transparently on first run. Once
// seeded, the DB is authoritative and this is a no-op.
func SeedSettingsIfEmpty(ctx context.Context, store SettingsStore, cfg Config) (bool, error) {
	existing, err := store.Settings(ctx)
	if err != nil {
		return false, fmt.Errorf("read settings: %w", err)
	}
	if len(existing) > 0 {
		return false, nil
	}
	for key, val := range SettingsFromConfig(cfg) {
		if err := store.SetSetting(ctx, key, val); err != nil {
			return false, fmt.Errorf("seed setting %q: %w", key, err)
		}
	}
	return true, nil
}
