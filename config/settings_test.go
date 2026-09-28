package config

import (
	"context"
	"maps"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// memSettings is an in-memory SettingsStore for tests.
type memSettings struct {
	m       map[string]string
	getErr  error
	setKeys []string // order of SetSetting calls
}

func newMemSettings() *memSettings { return &memSettings{m: map[string]string{}} }

func (s *memSettings) Settings(context.Context) (map[string]string, error) {
	if s.getErr != nil {
		return nil, s.getErr
	}
	out := make(map[string]string, len(s.m))
	maps.Copy(out, s.m)
	return out, nil
}

func (s *memSettings) SetSetting(_ context.Context, key, value string) error {
	s.m[key] = value
	s.setKeys = append(s.setKeys, key)
	return nil
}

func TestApplySettingsOverlaysConfig(t *testing.T) {
	cfg := Default()
	err := applySettings(&cfg, map[string]string{
		KeyAddr:               "ws://overlay:9002",
		KeyStaleThreshold:     "12s",
		KeyDeviceListInterval: "90s",
		KeyDeviceListPassword: "abc12",
		"unknown_key":         "ignored",
	})
	require.NoError(t, err)
	assert.Equal(t, "ws://overlay:9002", cfg.Addr)
	assert.Equal(t, 12*time.Second, cfg.StaleThreshold.Duration())
	assert.Equal(t, 90*time.Second, cfg.DeviceList.Interval.Duration())
	assert.Equal(t, "abc12", cfg.DeviceList.Password)
	// Keys not present are left at their default.
	assert.Equal(t, Default().ReconnectMaxInterval, cfg.ReconnectMaxInterval)
}

func TestApplySettingsBadDuration(t *testing.T) {
	cfg := Default()
	err := applySettings(&cfg, map[string]string{KeyStaleThreshold: "not-a-duration"})
	require.Error(t, err)
}

func TestSettingsFromConfigRoundTrip(t *testing.T) {
	cfg := Default()
	cfg.Addr = "ws://rt:9002"
	cfg.DeviceList.Password = "xyz99"
	cfg.Timezone = "Europe/Stockholm"

	m := SettingsFromConfig(cfg)

	var got Config
	require.NoError(t, applySettings(&got, m))
	assert.Equal(t, cfg.Addr, got.Addr)
	assert.Equal(t, cfg.StaleThreshold, got.StaleThreshold)
	assert.Equal(t, cfg.DeviceList.Interval, got.DeviceList.Interval)
	assert.Equal(t, cfg.DeviceList.Password, got.DeviceList.Password)
	assert.Equal(t, cfg.Timezone, got.Timezone)
}

func TestSeedSettingsIfEmpty(t *testing.T) {
	store := newMemSettings()
	cfg := Default()
	cfg.Addr = "ws://seed:9002"

	seeded, err := SeedSettingsIfEmpty(context.Background(), store, cfg)
	require.NoError(t, err)
	assert.True(t, seeded)
	assert.Equal(t, "ws://seed:9002", store.m[KeyAddr])

	// Second call is a no-op because the table is now non-empty.
	before := len(store.setKeys)
	seeded, err = SeedSettingsIfEmpty(context.Background(), store, cfg)
	require.NoError(t, err)
	assert.False(t, seeded)
	assert.Equal(t, before, len(store.setKeys))
}

func TestSeedMissingSettings(t *testing.T) {
	store := newMemSettings()
	// Simulate an existing install: settings already present, but from before
	// KeyTimezone existed.
	store.m[KeyAddr] = "ws://existing:9002"

	cfg := Default()
	cfg.Addr = "ws://from-file:9002" // present in store, must not be overwritten
	cfg.Timezone = "Europe/Stockholm"

	err := SeedMissingSettings(context.Background(), store, cfg)
	require.NoError(t, err)
	assert.Equal(t, "ws://existing:9002", store.m[KeyAddr], "existing key must be left untouched")
	assert.Equal(t, "Europe/Stockholm", store.m[KeyTimezone], "missing key must be backfilled")

	// Second call is a no-op: the key is now present.
	before := len(store.setKeys)
	cfg.Timezone = "America/Los_Angeles"
	require.NoError(t, SeedMissingSettings(context.Background(), store, cfg))
	assert.Equal(t, before, len(store.setKeys))
	assert.Equal(t, "Europe/Stockholm", store.m[KeyTimezone])
}

func TestLoadWithStoreOverlaysFile(t *testing.T) {
	store := newMemSettings()
	store.m[KeyAddr] = "ws://from-db:9002"

	// Non-existent file path → defaults, then DB overlay wins.
	cfg, err := LoadWithStore(context.Background(), "/no/such/config.yaml", store)
	require.NoError(t, err)
	assert.Equal(t, "ws://from-db:9002", cfg.Addr)
}
