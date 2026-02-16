package config_test

import (
	"encoding/json"
	"testing"

	"github.com/ruminaider/claude-sync/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// assertHookHasCommand checks that a json.RawMessage hook entry contains
// the expected command string in its first hook entry.
func assertHookHasCommand(t *testing.T, hookData json.RawMessage, expectedCmd string) {
	t.Helper()
	var entries []struct {
		Hooks []struct {
			Command string `json:"command"`
		} `json:"hooks"`
	}
	require.NoError(t, json.Unmarshal(hookData, &entries))
	require.NotEmpty(t, entries)
	require.NotEmpty(t, entries[0].Hooks)
	assert.Equal(t, expectedCmd, entries[0].Hooks[0].Command)
}

// makeHookJSON builds a json.RawMessage for a single-command hook.
func makeHookJSON(command string) json.RawMessage {
	return config.ExpandHookCommand(command)
}

func TestParseConfig(t *testing.T) {
	t.Run("categorized format", func(t *testing.T) {
		input := []byte(`version: "1.0.0"
plugins:
  upstream:
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
		assertHookHasCommand(t, cfg.Hooks["PreCompact"], "bd prime")
	})

	t.Run("empty config", func(t *testing.T) {
		input := []byte(`version: "1.0.0"
plugins:
  upstream: []
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
		Hooks: map[string]json.RawMessage{
			"SessionStart": makeHookJSON("claude-sync pull --quiet"),
		},
	}

	data, err := config.Marshal(cfg)
	require.NoError(t, err)

	parsed, err := config.Parse(data)
	require.NoError(t, err)
	assert.Equal(t, cfg.Version, parsed.Version)
	assert.Equal(t, cfg.Upstream, parsed.Upstream)
	assertHookHasCommand(t, parsed.Hooks["SessionStart"], "claude-sync pull --quiet")
}

func TestParseConfigV2(t *testing.T) {
	input := []byte(`version: "1.0.0"
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
	assert.Equal(t, "1.0.0", cfg.Version)
	assert.Equal(t, []string{"context7@claude-plugins-official", "playwright@claude-plugins-official"}, cfg.Upstream)
	assert.Equal(t, "0.44.0", cfg.Pinned["beads@beads-marketplace"])
	assert.Equal(t, []string{"compound-engineering-django-ts", "figma-minimal"}, cfg.Forked)
	assert.Equal(t, "opus", cfg.Settings["model"])
}

func TestMarshalConfigV2(t *testing.T) {
	cfg := config.Config{
		Version:  "1.0.0",
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
	assert.Equal(t, "1.0.0", parsed.Version)
	assert.Equal(t, cfg.Upstream, parsed.Upstream)
	assert.Equal(t, cfg.Pinned, parsed.Pinned)
	assert.Equal(t, cfg.Forked, parsed.Forked)
	assert.Equal(t, "opus", parsed.Settings["model"])
}

func TestMarshalRoundTrip(t *testing.T) {
	cfg := config.Config{
		Version:  "1.0.0",
		Upstream: []string{"a@b"},
		Pinned:   map[string]string{"c@d": "1.0"},
		Forked:   []string{"e"},
	}
	data, err := config.Marshal(cfg)
	require.NoError(t, err)

	parsed, err := config.Parse(data)
	require.NoError(t, err)
	assert.Equal(t, "1.0.0", parsed.Version)
	assert.Equal(t, cfg.Upstream, parsed.Upstream)
	assert.Equal(t, cfg.Pinned, parsed.Pinned)
	assert.Equal(t, cfg.Forked, parsed.Forked)
}

func TestAllPluginKeys(t *testing.T) {
	cfg := config.Config{
		Version:  "1.0.0",
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
		Version:  "1.0.0",
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
		Version: "1.0.0",
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

func TestShouldSkip(t *testing.T) {
	prefs := config.UserPreferences{
		Sync: config.SyncPrefs{Skip: []string{"hooks"}},
	}
	assert.True(t, prefs.ShouldSkip(config.CategoryHooks))
	assert.False(t, prefs.ShouldSkip(config.CategorySettings))
}

func TestShouldSkip_Empty(t *testing.T) {
	prefs := config.DefaultUserPreferences()
	assert.False(t, prefs.ShouldSkip(config.CategoryHooks))
	assert.False(t, prefs.ShouldSkip(config.CategorySettings))
}

func TestMarshalUserPreferences(t *testing.T) {
	prefs := config.UserPreferences{
		SyncMode: "union",
		Sync:     config.SyncPrefs{Skip: []string{"hooks"}},
	}
	data, err := config.MarshalUserPreferences(prefs)
	require.NoError(t, err)

	parsed, err := config.ParseUserPreferences(data)
	require.NoError(t, err)
	assert.Equal(t, "union", parsed.SyncMode)
	assert.True(t, parsed.ShouldSkip(config.CategoryHooks))
	assert.False(t, parsed.ShouldSkip(config.CategorySettings))
}

func TestParseConfig_BackwardCompatHooks(t *testing.T) {
	t.Run("old format plain string is expanded", func(t *testing.T) {
		input := []byte(`version: "1.0.0"
plugins:
  upstream: []
hooks:
  PreCompact: "bd prime"
`)
		cfg, err := config.Parse(input)
		require.NoError(t, err)
		assertHookHasCommand(t, cfg.Hooks["PreCompact"], "bd prime")
	})

	t.Run("new format JSON string is preserved", func(t *testing.T) {
		input := []byte(`version: "1.0.0"
plugins:
  upstream: []
hooks:
  PreCompact: '[{"matcher":"","hooks":[{"type":"command","command":"bd prime"}]}]'
`)
		cfg, err := config.Parse(input)
		require.NoError(t, err)
		assertHookHasCommand(t, cfg.Hooks["PreCompact"], "bd prime")
	})

	t.Run("round-trip preserves raw JSON", func(t *testing.T) {
		rawJSON := `[{"matcher":"proj/.*","hooks":[{"type":"command","command":"lint"},{"type":"command","command":"test"}]}]`
		cfg := config.Config{
			Version:  "1.0.0",
			Upstream: []string{"a@b"},
			Pinned:   map[string]string{},
			Hooks: map[string]json.RawMessage{
				"PreCompact": json.RawMessage(rawJSON),
			},
		}
		data, err := config.Marshal(cfg)
		require.NoError(t, err)

		parsed, err := config.Parse(data)
		require.NoError(t, err)
		assert.JSONEq(t, rawJSON, string(parsed.Hooks["PreCompact"]))
	})
}

func TestParseConfig_Permissions(t *testing.T) {
	t.Run("parse with permissions", func(t *testing.T) {
		input := []byte(`version: "1.0.0"
plugins:
  upstream: []
permissions:
  allow:
    - Bash(git *)
    - Read
  deny:
    - Bash(rm *)
`)
		cfg, err := config.Parse(input)
		require.NoError(t, err)
		assert.Equal(t, []string{"Bash(git *)", "Read"}, cfg.Permissions.Allow)
		assert.Equal(t, []string{"Bash(rm *)"}, cfg.Permissions.Deny)
	})

	t.Run("parse without permissions returns empty", func(t *testing.T) {
		input := []byte(`version: "1.0.0"
plugins:
  upstream: []
`)
		cfg, err := config.Parse(input)
		require.NoError(t, err)
		assert.Empty(t, cfg.Permissions.Allow)
		assert.Empty(t, cfg.Permissions.Deny)
	})

	t.Run("marshal round-trip", func(t *testing.T) {
		cfg := config.Config{
			Version: "1.0.0",
			Pinned:  map[string]string{},
			Permissions: config.Permissions{
				Allow: []string{"Bash(git *)", "Read"},
				Deny:  []string{"Bash(rm *)"},
			},
		}
		data, err := config.Marshal(cfg)
		require.NoError(t, err)

		parsed, err := config.Parse(data)
		require.NoError(t, err)
		assert.Equal(t, cfg.Permissions.Allow, parsed.Permissions.Allow)
		assert.Equal(t, cfg.Permissions.Deny, parsed.Permissions.Deny)
	})
}

func TestParseConfig_ClaudeMD(t *testing.T) {
	t.Run("parse with claude_md", func(t *testing.T) {
		input := []byte(`version: "1.0.0"
plugins:
  upstream: []
claude_md:
  include:
    - shared/coding-standards.md
    - shared/api-patterns.md
`)
		cfg, err := config.Parse(input)
		require.NoError(t, err)
		assert.Equal(t, []string{"shared/coding-standards.md", "shared/api-patterns.md"}, cfg.ClaudeMD.Include)
	})

	t.Run("parse without claude_md returns empty", func(t *testing.T) {
		input := []byte(`version: "1.0.0"
plugins:
  upstream: []
`)
		cfg, err := config.Parse(input)
		require.NoError(t, err)
		assert.Empty(t, cfg.ClaudeMD.Include)
	})

	t.Run("marshal round-trip", func(t *testing.T) {
		cfg := config.Config{
			Version: "1.0.0",
			Pinned:  map[string]string{},
			ClaudeMD: config.ClaudeMDConfig{
				Include: []string{"shared/coding-standards.md"},
			},
		}
		data, err := config.Marshal(cfg)
		require.NoError(t, err)

		parsed, err := config.Parse(data)
		require.NoError(t, err)
		assert.Equal(t, cfg.ClaudeMD.Include, parsed.ClaudeMD.Include)
	})
}

func TestParseConfig_MCP(t *testing.T) {
	t.Run("parse with mcp", func(t *testing.T) {
		input := []byte(`version: "1.0.0"
plugins:
  upstream: []
mcp:
  postgres:
    command: npx
    args:
      - -y
      - "@modelcontextprotocol/server-postgres"
`)
		cfg, err := config.Parse(input)
		require.NoError(t, err)
		require.Contains(t, cfg.MCP, "postgres")

		var val map[string]any
		require.NoError(t, json.Unmarshal(cfg.MCP["postgres"], &val))
		assert.Equal(t, "npx", val["command"])
	})

	t.Run("marshal round-trip", func(t *testing.T) {
		mcpData, _ := json.Marshal(map[string]any{"command": "npx", "args": []string{"-y", "server"}})
		cfg := config.Config{
			Version: "1.0.0",
			Pinned:  map[string]string{},
			MCP: map[string]json.RawMessage{
				"myserver": json.RawMessage(mcpData),
			},
		}
		data, err := config.Marshal(cfg)
		require.NoError(t, err)

		parsed, err := config.Parse(data)
		require.NoError(t, err)
		require.Contains(t, parsed.MCP, "myserver")

		var val map[string]any
		require.NoError(t, json.Unmarshal(parsed.MCP["myserver"], &val))
		assert.Equal(t, "npx", val["command"])
	})
}

func TestParseConfig_Keybindings(t *testing.T) {
	t.Run("parse with keybindings", func(t *testing.T) {
		input := []byte(`version: "1.0.0"
plugins:
  upstream: []
keybindings:
  ctrl+k: clear
  ctrl+l: redraw
`)
		cfg, err := config.Parse(input)
		require.NoError(t, err)
		assert.Equal(t, "clear", cfg.Keybindings["ctrl+k"])
		assert.Equal(t, "redraw", cfg.Keybindings["ctrl+l"])
	})

	t.Run("marshal round-trip", func(t *testing.T) {
		cfg := config.Config{
			Version:     "1.0.0",
			Pinned:      map[string]string{},
			Keybindings: map[string]any{"ctrl+k": "clear"},
		}
		data, err := config.Marshal(cfg)
		require.NoError(t, err)

		parsed, err := config.Parse(data)
		require.NoError(t, err)
		assert.Equal(t, "clear", parsed.Keybindings["ctrl+k"])
	})
}

func TestParseUserPreferences_WithSync(t *testing.T) {
	input := []byte(`sync_mode: union
sync:
  skip:
    - settings
    - hooks
`)
	prefs, err := config.ParseUserPreferences(input)
	require.NoError(t, err)
	assert.True(t, prefs.ShouldSkip(config.CategorySettings))
	assert.True(t, prefs.ShouldSkip(config.CategoryHooks))
}
