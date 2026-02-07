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
		assert.Len(t, cfg.Upstream, 3)
		assert.Contains(t, cfg.Upstream, "context7@claude-plugins-official")
		assert.Equal(t, "opus", cfg.Settings["model"])
		assert.Equal(t, "bd prime", cfg.Hooks["PreCompact"])
	})

	t.Run("empty config", func(t *testing.T) {
		input := []byte(`version: "1.0.0"
plugins: []
`)
		cfg, err := config.Parse(input)
		require.NoError(t, err)
		assert.Empty(t, cfg.Upstream)
	})

	t.Run("invalid yaml", func(t *testing.T) {
		_, err := config.Parse([]byte(`{{{`))
		assert.Error(t, err)
	})
}

func TestMarshalConfig(t *testing.T) {
	cfg := config.Config{
		Version:  "1.0.0",
		Upstream: []string{"context7@claude-plugins-official"},
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
	assert.Equal(t, cfg.Upstream, parsed.Upstream)
}

func TestParseConfigV2(t *testing.T) {
	input := []byte(`version: "2.0.0"
plugins:
  upstream:
    - context7@claude-plugins-official
    - playwright@claude-plugins-official
  pinned:
    - beads@beads-marketplace: "0.44.0"
  forked:
    - compound-engineering-django-ts
    - figma-minimal
settings:
  model: opus
`)
	cfg, err := config.Parse(input)
	require.NoError(t, err)
	assert.Equal(t, "2.0.0", cfg.Version)
	assert.Equal(t, []string{"context7@claude-plugins-official", "playwright@claude-plugins-official"}, cfg.Upstream)
	assert.Equal(t, "0.44.0", cfg.Pinned["beads@beads-marketplace"])
	assert.Equal(t, []string{"compound-engineering-django-ts", "figma-minimal"}, cfg.Forked)
	assert.Equal(t, "opus", cfg.Settings["model"])
}

func TestParseConfigV1Compat(t *testing.T) {
	input := []byte(`version: "1.0.0"
plugins:
  - context7@claude-plugins-official
  - beads@beads-marketplace
  - episodic-memory@superpowers-marketplace
`)
	cfg, err := config.Parse(input)
	require.NoError(t, err)
	assert.Equal(t, "1.0.0", cfg.Version)
	assert.Equal(t, []string{
		"context7@claude-plugins-official",
		"beads@beads-marketplace",
		"episodic-memory@superpowers-marketplace",
	}, cfg.Upstream)
	assert.Empty(t, cfg.Pinned)
	assert.Empty(t, cfg.Forked)
}

func TestMarshalConfigV2(t *testing.T) {
	cfg := config.Config{
		Version:  "2.0.0",
		Upstream: []string{"context7@claude-plugins-official", "playwright@claude-plugins-official"},
		Pinned:   map[string]string{"beads@beads-marketplace": "0.44.0"},
		Forked:   []string{"figma-minimal"},
		Settings: map[string]any{"model": "opus"},
	}

	data, err := config.MarshalV2(cfg)
	require.NoError(t, err)

	// Round-trip: parse back and verify
	parsed, err := config.Parse(data)
	require.NoError(t, err)
	assert.Equal(t, "2.0.0", parsed.Version)
	assert.Equal(t, cfg.Upstream, parsed.Upstream)
	assert.Equal(t, cfg.Pinned, parsed.Pinned)
	assert.Equal(t, cfg.Forked, parsed.Forked)
	assert.Equal(t, "opus", parsed.Settings["model"])
}

func TestMarshalAutoSelectsFormat(t *testing.T) {
	t.Run("v1 uses flat format", func(t *testing.T) {
		cfg := config.Config{
			Version:  "1.0.0",
			Upstream: []string{"a@b", "c@d"},
		}
		data, err := config.Marshal(cfg)
		require.NoError(t, err)

		parsed, err := config.Parse(data)
		require.NoError(t, err)
		assert.Equal(t, "1.0.0", parsed.Version)
		assert.Equal(t, cfg.Upstream, parsed.Upstream)
	})

	t.Run("v2 uses categorized format", func(t *testing.T) {
		cfg := config.Config{
			Version:  "2.0.0",
			Upstream: []string{"a@b"},
			Pinned:   map[string]string{"c@d": "1.0"},
			Forked:   []string{"e"},
		}
		data, err := config.Marshal(cfg)
		require.NoError(t, err)

		parsed, err := config.Parse(data)
		require.NoError(t, err)
		assert.Equal(t, "2.0.0", parsed.Version)
		assert.Equal(t, cfg.Upstream, parsed.Upstream)
		assert.Equal(t, cfg.Pinned, parsed.Pinned)
		assert.Equal(t, cfg.Forked, parsed.Forked)
	})
}

func TestAllPluginKeys(t *testing.T) {
	cfg := config.Config{
		Version:  "2.0.0",
		Upstream: []string{"context7@claude-plugins-official", "playwright@claude-plugins-official"},
		Pinned:   map[string]string{"beads@beads-marketplace": "0.44.0"},
		Forked:   []string{"figma-minimal"},
	}

	keys := cfg.AllPluginKeys()
	assert.Len(t, keys, 4)
	assert.Contains(t, keys, "context7@claude-plugins-official")
	assert.Contains(t, keys, "playwright@claude-plugins-official")
	assert.Contains(t, keys, "beads@beads-marketplace")
	assert.Contains(t, keys, "figma-minimal")

	// Verify sorted
	for i := 1; i < len(keys); i++ {
		assert.True(t, keys[i-1] <= keys[i], "keys should be sorted: %s > %s", keys[i-1], keys[i])
	}
}

func TestAllPluginKeys_NoDuplicates(t *testing.T) {
	cfg := config.Config{
		Version:  "2.0.0",
		Upstream: []string{"context7@claude-plugins-official"},
		Pinned:   map[string]string{"context7@claude-plugins-official": "1.0"},
		Forked:   []string{},
	}

	keys := cfg.AllPluginKeys()
	assert.Len(t, keys, 1)
	assert.Equal(t, "context7@claude-plugins-official", keys[0])
}

func TestAllPluginKeys_EmptyConfig(t *testing.T) {
	cfg := config.Config{
		Version: "2.0.0",
		Pinned:  map[string]string{},
	}

	keys := cfg.AllPluginKeys()
	assert.Empty(t, keys)
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
