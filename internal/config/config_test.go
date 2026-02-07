package config_test

import (
	"testing"

	"github.com/ruminaider/claude-sync/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseConfig(t *testing.T) {
	t.Run("phase 1 simple format", func(t *testing.T) {
		input := []byte(`version: "1.0.0"
plugins:
  - context7@claude-plugins-official
  - episodic-memory@superpowers-marketplace
  - beads@beads-marketplace
settings:
  model: opus
hooks:
  PreCompact: "bd prime"
  SessionStart: "claude-sync pull --quiet"
`)
		cfg, err := config.Parse(input)
		require.NoError(t, err)
		assert.Equal(t, "1.0.0", cfg.Version)
		assert.Len(t, cfg.Plugins, 3)
		assert.Contains(t, cfg.Plugins, "context7@claude-plugins-official")
		assert.Equal(t, "opus", cfg.Settings["model"])
		assert.Equal(t, "bd prime", cfg.Hooks["PreCompact"])
	})

	t.Run("empty config", func(t *testing.T) {
		input := []byte(`version: "1.0.0"
plugins: []
`)
		cfg, err := config.Parse(input)
		require.NoError(t, err)
		assert.Empty(t, cfg.Plugins)
	})

	t.Run("invalid yaml", func(t *testing.T) {
		_, err := config.Parse([]byte(`{{{`))
		assert.Error(t, err)
	})
}

func TestMarshalConfig(t *testing.T) {
	cfg := config.Config{
		Version: "1.0.0",
		Plugins: []string{"context7@claude-plugins-official"},
		Settings: map[string]any{
			"model": "opus",
		},
		Hooks: map[string]string{
			"SessionStart": "claude-sync pull --quiet",
		},
	}

	data, err := config.Marshal(cfg)
	require.NoError(t, err)

	parsed, err := config.Parse(data)
	require.NoError(t, err)
	assert.Equal(t, cfg.Version, parsed.Version)
	assert.Equal(t, cfg.Plugins, parsed.Plugins)
}

func TestParseUserPreferences(t *testing.T) {
	input := []byte(`sync_mode: union
settings:
  model: opus
plugins:
  unsubscribe:
    - ralph-wiggum
    - greptile
  personal:
    - some-niche-plugin
pins:
  episodic-memory: "1.0.15"
`)
	prefs, err := config.ParseUserPreferences(input)
	require.NoError(t, err)
	assert.Equal(t, "union", prefs.SyncMode)
	assert.Contains(t, prefs.Plugins.Unsubscribe, "ralph-wiggum")
	assert.Contains(t, prefs.Plugins.Personal, "some-niche-plugin")
	assert.Equal(t, "1.0.15", prefs.Pins["episodic-memory"])
}

func TestDefaultUserPreferences(t *testing.T) {
	prefs := config.DefaultUserPreferences()
	assert.Equal(t, "union", prefs.SyncMode)
	assert.Empty(t, prefs.Plugins.Unsubscribe)
}
