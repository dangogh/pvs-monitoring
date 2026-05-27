package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefault(t *testing.T) {
	cfg := Default()
	assert.Equal(t, defaultAddr, cfg.Addr)
	assert.Equal(t, time.Second, cfg.ReconnectInitialInterval.Duration())
	assert.Equal(t, 30*time.Second, cfg.ReconnectMaxInterval.Duration())
	assert.Equal(t, 5*time.Second, cfg.StaleThreshold.Duration())
	assert.Equal(t, "http://sunpowerconsole.com", cfg.DeviceList.URL)
	assert.Equal(t, 60*time.Second, cfg.DeviceList.Interval.Duration())
	assert.Equal(t, "ssm_owner", cfg.DeviceList.Username)
	assert.Empty(t, cfg.DeviceList.Password)
}

func TestLoadMissingFile(t *testing.T) {
	cfg, err := Load(filepath.Join(t.TempDir(), "nonexistent.yaml"))
	require.NoError(t, err)
	assert.Equal(t, Default(), cfg)
}

func TestLoadOverridesDefaults(t *testing.T) {
	yaml := `
addr: ws://10.0.0.1:9002
reconnect_initial_interval: 2s
reconnect_max_interval: 60s
stale_threshold: 10s
device_list:
  url: http://10.0.0.1
  interval: 30s
  username: admin
  password: abc12
`
	path := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte(yaml), 0o600))

	cfg, err := Load(path)
	require.NoError(t, err)
	assert.Equal(t, "ws://10.0.0.1:9002", cfg.Addr)
	assert.Equal(t, 2*time.Second, cfg.ReconnectInitialInterval.Duration())
	assert.Equal(t, 60*time.Second, cfg.ReconnectMaxInterval.Duration())
	assert.Equal(t, 10*time.Second, cfg.StaleThreshold.Duration())
	assert.Equal(t, "http://10.0.0.1", cfg.DeviceList.URL)
	assert.Equal(t, 30*time.Second, cfg.DeviceList.Interval.Duration())
	assert.Equal(t, "admin", cfg.DeviceList.Username)
	assert.Equal(t, "abc12", cfg.DeviceList.Password)
}

func TestLoadPartialOverride(t *testing.T) {
	yaml := `addr: ws://10.0.0.2:9002`
	path := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte(yaml), 0o600))

	cfg, err := Load(path)
	require.NoError(t, err)
	assert.Equal(t, "ws://10.0.0.2:9002", cfg.Addr)
	// Unspecified fields keep defaults.
	assert.Equal(t, time.Second, cfg.ReconnectInitialInterval.Duration())
	assert.Equal(t, "ssm_owner", cfg.DeviceList.Username)
}

func TestLoadInvalidYAML(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte(`{not: valid: yaml:`), 0o600))

	_, err := Load(path)
	assert.Error(t, err)
}

func TestLoadInvalidDuration(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte(`stale_threshold: "notaduration"`), 0o600))

	_, err := Load(path)
	assert.Error(t, err)
}

func TestDefaultPath(t *testing.T) {
	t.Run("uses XDG_CONFIG_HOME when set", func(t *testing.T) {
		t.Setenv("XDG_CONFIG_HOME", "/custom/config")
		assert.Equal(t, "/custom/config/pvs-monitor/config.yaml", DefaultPath())
	})

	t.Run("falls back to home dir", func(t *testing.T) {
		t.Setenv("XDG_CONFIG_HOME", "")
		home, _ := os.UserHomeDir()
		assert.Equal(t, filepath.Join(home, ".config", "pvs-monitor", "config.yaml"), DefaultPath())
	})
}

func TestDurationUnmarshalYAML(t *testing.T) {
	tests := []struct {
		name    string
		yaml    string
		want    time.Duration
		wantErr bool
	}{
		{name: "seconds", yaml: `stale_threshold: 5s`, want: 5 * time.Second},
		{name: "minutes", yaml: `stale_threshold: 2m`, want: 2 * time.Minute},
		{name: "milliseconds", yaml: `stale_threshold: 500ms`, want: 500 * time.Millisecond},
		{name: "invalid", yaml: `stale_threshold: "banana"`, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.yaml")
			require.NoError(t, os.WriteFile(path, []byte(tt.yaml), 0o600))
			cfg, err := Load(path)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, cfg.StaleThreshold.Duration())
		})
	}
}
