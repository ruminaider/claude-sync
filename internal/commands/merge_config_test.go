package commands_test

import (
	"encoding/json"
	"testing"

	"github.com/ruminaider/claude-sync/internal/commands"
	"github.com/ruminaider/claude-sync/internal/config"
	plugins "github.com/ruminaider/claude-sync/internal/plugins"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMergeExistingConfig_UpstreamPlugins(t *testing.T) {
	t.Run("injects missing upstream plugins", func(t *testing.T) {
		scan := &commands.InitScanResult{
			PluginKeys: []string{"existing@mkt"},
			Upstream:   []string{"existing@mkt"},
		}
		cfg := &config.Config{
			Upstream: []string{"existing@mkt", "new-one@mkt"},
		}

		commands.MergeExistingConfig(scan, cfg, "")

		assert.Contains(t, scan.PluginKeys, "new-one@mkt")
		assert.Contains(t, scan.Upstream, "new-one@mkt")
		assert.True(t, scan.ConfigOnly["new-one@mkt"])
	})

	t.Run("does not duplicate existing upstream plugins", func(t *testing.T) {
		scan := &commands.InitScanResult{
			PluginKeys: []string{"existing@mkt"},
			Upstream:   []string{"existing@mkt"},
		}
		cfg := &config.Config{
			Upstream: []string{"existing@mkt"},
		}

		commands.MergeExistingConfig(scan, cfg, "")

		assert.Equal(t, []string{"existing@mkt"}, scan.PluginKeys)
		assert.Equal(t, []string{"existing@mkt"}, scan.Upstream)
		assert.Empty(t, scan.ConfigOnly)
	})
}

func TestMergeExistingConfig_ForkedPlugins(t *testing.T) {
	t.Run("injects missing forked plugins with marketplace suffix", func(t *testing.T) {
		scan := &commands.InitScanResult{}
		cfg := &config.Config{
			Forked: []string{"my-fork"},
		}

		commands.MergeExistingConfig(scan, cfg, "")

		expectedKey := "my-fork@" + plugins.MarketplaceName
		assert.Contains(t, scan.PluginKeys, expectedKey)
		assert.Contains(t, scan.AutoForked, expectedKey)
		assert.True(t, scan.ConfigOnly[expectedKey])
	})

	t.Run("does not duplicate existing forked plugins", func(t *testing.T) {
		existingKey := "my-fork@" + plugins.MarketplaceName
		scan := &commands.InitScanResult{
			PluginKeys: []string{existingKey},
			AutoForked: []string{existingKey},
		}
		cfg := &config.Config{
			Forked: []string{"my-fork"},
		}

		commands.MergeExistingConfig(scan, cfg, "")

		assert.Equal(t, []string{existingKey}, scan.PluginKeys)
		assert.Equal(t, []string{existingKey}, scan.AutoForked)
		assert.Empty(t, scan.ConfigOnly)
	})
}

func TestMergeExistingConfig_Settings(t *testing.T) {
	t.Run("injects missing settings", func(t *testing.T) {
		scan := &commands.InitScanResult{
			Settings: map[string]any{"existing": "val1"},
		}
		cfg := &config.Config{
			Settings: map[string]any{"existing": "val1", "newKey": "val2"},
		}

		commands.MergeExistingConfig(scan, cfg, "")

		assert.Equal(t, "val2", scan.Settings["newKey"])
		assert.True(t, scan.ConfigOnly["newKey"])
		assert.False(t, scan.ConfigOnly["existing"])
	})

	t.Run("initializes nil settings map", func(t *testing.T) {
		scan := &commands.InitScanResult{}
		cfg := &config.Config{
			Settings: map[string]any{"theme": "dark"},
		}

		commands.MergeExistingConfig(scan, cfg, "")

		require.NotNil(t, scan.Settings)
		assert.Equal(t, "dark", scan.Settings["theme"])
		assert.True(t, scan.ConfigOnly["theme"])
	})

	t.Run("no-op when config settings is empty", func(t *testing.T) {
		scan := &commands.InitScanResult{}
		cfg := &config.Config{}

		commands.MergeExistingConfig(scan, cfg, "")

		assert.Nil(t, scan.Settings)
	})
}

func TestMergeExistingConfig_Hooks(t *testing.T) {
	t.Run("injects missing hooks", func(t *testing.T) {
		existing := json.RawMessage(`{"type":"command","command":"echo hi"}`)
		scan := &commands.InitScanResult{
			Hooks: map[string]json.RawMessage{"PreToolUse": existing},
		}
		newHook := json.RawMessage(`{"type":"command","command":"echo bye"}`)
		cfg := &config.Config{
			Hooks: map[string]json.RawMessage{"PreToolUse": existing, "PostToolUse": newHook},
		}

		commands.MergeExistingConfig(scan, cfg, "")

		assert.Equal(t, newHook, scan.Hooks["PostToolUse"])
		assert.True(t, scan.ConfigOnly["PostToolUse"])
		assert.False(t, scan.ConfigOnly["PreToolUse"])
	})

	t.Run("initializes nil hooks map", func(t *testing.T) {
		scan := &commands.InitScanResult{}
		hook := json.RawMessage(`{"type":"command","command":"test"}`)
		cfg := &config.Config{
			Hooks: map[string]json.RawMessage{"SessionEnd": hook},
		}

		commands.MergeExistingConfig(scan, cfg, "")

		require.NotNil(t, scan.Hooks)
		assert.Equal(t, hook, scan.Hooks["SessionEnd"])
		assert.True(t, scan.ConfigOnly["SessionEnd"])
	})

	t.Run("no-op when config hooks is empty", func(t *testing.T) {
		scan := &commands.InitScanResult{}
		cfg := &config.Config{}

		commands.MergeExistingConfig(scan, cfg, "")

		assert.Nil(t, scan.Hooks)
	})
}

func TestMergeExistingConfig_Permissions(t *testing.T) {
	t.Run("injects missing allow rules", func(t *testing.T) {
		scan := &commands.InitScanResult{
			Permissions: config.Permissions{
				Allow: []string{"Bash(git *)"},
			},
		}
		cfg := &config.Config{
			Permissions: config.Permissions{
				Allow: []string{"Bash(git *)", "Bash(npm *)"},
			},
		}

		commands.MergeExistingConfig(scan, cfg, "")

		assert.Contains(t, scan.Permissions.Allow, "Bash(npm *)")
		assert.True(t, scan.ConfigOnly["allow:Bash(npm *)"])
		assert.False(t, scan.ConfigOnly["allow:Bash(git *)"])
	})

	t.Run("injects missing deny rules", func(t *testing.T) {
		scan := &commands.InitScanResult{}
		cfg := &config.Config{
			Permissions: config.Permissions{
				Deny: []string{"Bash(rm -rf *)"},
			},
		}

		commands.MergeExistingConfig(scan, cfg, "")

		assert.Contains(t, scan.Permissions.Deny, "Bash(rm -rf *)")
		assert.True(t, scan.ConfigOnly["deny:Bash(rm -rf *)"])
	})

	t.Run("does not duplicate existing rules", func(t *testing.T) {
		scan := &commands.InitScanResult{
			Permissions: config.Permissions{
				Allow: []string{"Bash(git *)"},
				Deny:  []string{"Bash(rm -rf *)"},
			},
		}
		cfg := &config.Config{
			Permissions: config.Permissions{
				Allow: []string{"Bash(git *)"},
				Deny:  []string{"Bash(rm -rf *)"},
			},
		}

		commands.MergeExistingConfig(scan, cfg, "")

		assert.Len(t, scan.Permissions.Allow, 1)
		assert.Len(t, scan.Permissions.Deny, 1)
		assert.Empty(t, scan.ConfigOnly)
	})

	t.Run("handles both allow and deny together", func(t *testing.T) {
		scan := &commands.InitScanResult{}
		cfg := &config.Config{
			Permissions: config.Permissions{
				Allow: []string{"Bash(git *)"},
				Deny:  []string{"Bash(rm *)"},
			},
		}

		commands.MergeExistingConfig(scan, cfg, "")

		assert.Equal(t, []string{"Bash(git *)"}, scan.Permissions.Allow)
		assert.Equal(t, []string{"Bash(rm *)"}, scan.Permissions.Deny)
		assert.True(t, scan.ConfigOnly["allow:Bash(git *)"])
		assert.True(t, scan.ConfigOnly["deny:Bash(rm *)"])
	})
}

func TestMergeExistingConfig_MCP(t *testing.T) {
	t.Run("injects missing MCP servers", func(t *testing.T) {
		existingServer := json.RawMessage(`{"command":"existing"}`)
		scan := &commands.InitScanResult{
			MCP: map[string]json.RawMessage{"server-a": existingServer},
		}
		newServer := json.RawMessage(`{"command":"new-server"}`)
		cfg := &config.Config{
			MCP: map[string]json.RawMessage{"server-a": existingServer, "server-b": newServer},
		}

		commands.MergeExistingConfig(scan, cfg, "")

		assert.Equal(t, newServer, scan.MCP["server-b"])
		assert.True(t, scan.ConfigOnly["server-b"])
		assert.False(t, scan.ConfigOnly["server-a"])
	})

	t.Run("initializes nil MCP map", func(t *testing.T) {
		scan := &commands.InitScanResult{}
		srv := json.RawMessage(`{"command":"srv"}`)
		cfg := &config.Config{
			MCP: map[string]json.RawMessage{"my-mcp": srv},
		}

		commands.MergeExistingConfig(scan, cfg, "")

		require.NotNil(t, scan.MCP)
		assert.Equal(t, srv, scan.MCP["my-mcp"])
		assert.True(t, scan.ConfigOnly["my-mcp"])
	})

	t.Run("no-op when config MCP is empty", func(t *testing.T) {
		scan := &commands.InitScanResult{}
		cfg := &config.Config{}

		commands.MergeExistingConfig(scan, cfg, "")

		assert.Nil(t, scan.MCP)
	})
}

func TestMergeExistingConfig_Keybindings(t *testing.T) {
	t.Run("injects missing keybindings", func(t *testing.T) {
		scan := &commands.InitScanResult{
			Keybindings: map[string]any{"ctrl+s": "save"},
		}
		cfg := &config.Config{
			Keybindings: map[string]any{"ctrl+s": "save", "ctrl+q": "quit"},
		}

		commands.MergeExistingConfig(scan, cfg, "")

		assert.Equal(t, "quit", scan.Keybindings["ctrl+q"])
		assert.True(t, scan.ConfigOnly["ctrl+q"])
		assert.False(t, scan.ConfigOnly["ctrl+s"])
	})

	t.Run("initializes nil keybindings map", func(t *testing.T) {
		scan := &commands.InitScanResult{}
		cfg := &config.Config{
			Keybindings: map[string]any{"ctrl+z": "undo"},
		}

		commands.MergeExistingConfig(scan, cfg, "")

		require.NotNil(t, scan.Keybindings)
		assert.Equal(t, "undo", scan.Keybindings["ctrl+z"])
		assert.True(t, scan.ConfigOnly["ctrl+z"])
	})

	t.Run("no-op when config keybindings is empty", func(t *testing.T) {
		scan := &commands.InitScanResult{}
		cfg := &config.Config{}

		commands.MergeExistingConfig(scan, cfg, "")

		assert.Nil(t, scan.Keybindings)
	})
}

func TestMergeExistingConfig_ConfigOnlyTracking(t *testing.T) {
	t.Run("initializes ConfigOnly when nil", func(t *testing.T) {
		scan := &commands.InitScanResult{}
		cfg := &config.Config{
			Upstream: []string{"plugin@mkt"},
		}

		commands.MergeExistingConfig(scan, cfg, "")

		require.NotNil(t, scan.ConfigOnly)
		assert.True(t, scan.ConfigOnly["plugin@mkt"])
	})

	t.Run("preserves existing ConfigOnly entries", func(t *testing.T) {
		scan := &commands.InitScanResult{
			ConfigOnly: map[string]bool{"previous": true},
		}
		cfg := &config.Config{
			Settings: map[string]any{"newSetting": 42},
		}

		commands.MergeExistingConfig(scan, cfg, "")

		assert.True(t, scan.ConfigOnly["previous"])
		assert.True(t, scan.ConfigOnly["newSetting"])
	})

	t.Run("only marks injected items, not pre-existing ones", func(t *testing.T) {
		scan := &commands.InitScanResult{
			PluginKeys: []string{"local@mkt"},
			Upstream:   []string{"local@mkt"},
			Settings:   map[string]any{"localSetting": "yes"},
			Hooks:      map[string]json.RawMessage{"PreToolUse": json.RawMessage(`{}`)},
			Permissions: config.Permissions{
				Allow: []string{"Bash(git *)"},
			},
			MCP:         map[string]json.RawMessage{"local-mcp": json.RawMessage(`{}`)},
			Keybindings: map[string]any{"ctrl+s": "save"},
		}
		cfg := &config.Config{
			Upstream:    []string{"local@mkt", "remote@mkt"},
			Settings:    map[string]any{"localSetting": "yes", "remoteSetting": "no"},
			Hooks:       map[string]json.RawMessage{"PreToolUse": json.RawMessage(`{}`), "SessionEnd": json.RawMessage(`{}`)},
			Permissions: config.Permissions{Allow: []string{"Bash(git *)", "Bash(npm *)"}},
			MCP:         map[string]json.RawMessage{"local-mcp": json.RawMessage(`{}`), "remote-mcp": json.RawMessage(`{}`)},
			Keybindings: map[string]any{"ctrl+s": "save", "ctrl+q": "quit"},
		}

		commands.MergeExistingConfig(scan, cfg, "")

		// Injected items should be in ConfigOnly.
		assert.True(t, scan.ConfigOnly["remote@mkt"])
		assert.True(t, scan.ConfigOnly["remoteSetting"])
		assert.True(t, scan.ConfigOnly["SessionEnd"])
		assert.True(t, scan.ConfigOnly["allow:Bash(npm *)"])
		assert.True(t, scan.ConfigOnly["remote-mcp"])
		assert.True(t, scan.ConfigOnly["ctrl+q"])

		// Pre-existing items should NOT be in ConfigOnly.
		assert.False(t, scan.ConfigOnly["local@mkt"])
		assert.False(t, scan.ConfigOnly["localSetting"])
		assert.False(t, scan.ConfigOnly["PreToolUse"])
		assert.False(t, scan.ConfigOnly["allow:Bash(git *)"])
		assert.False(t, scan.ConfigOnly["local-mcp"])
		assert.False(t, scan.ConfigOnly["ctrl+s"])
	})
}

func TestMergeExistingConfig_EmptyScan(t *testing.T) {
	t.Run("all sections injected into empty scan", func(t *testing.T) {
		scan := &commands.InitScanResult{}
		cfg := &config.Config{
			Upstream:    []string{"p1@mkt"},
			Forked:      []string{"f1"},
			Settings:    map[string]any{"k": "v"},
			Hooks:       map[string]json.RawMessage{"H": json.RawMessage(`{}`)},
			Permissions: config.Permissions{Allow: []string{"A"}, Deny: []string{"D"}},
			MCP:         map[string]json.RawMessage{"M": json.RawMessage(`{}`)},
			Keybindings: map[string]any{"B": "bind"},
		}

		commands.MergeExistingConfig(scan, cfg, "")

		forkedKey := "f1@" + plugins.MarketplaceName

		assert.ElementsMatch(t, []string{"p1@mkt", forkedKey}, scan.PluginKeys)
		assert.Equal(t, []string{"p1@mkt"}, scan.Upstream)
		assert.Equal(t, []string{forkedKey}, scan.AutoForked)
		assert.Equal(t, "v", scan.Settings["k"])
		assert.Equal(t, json.RawMessage(`{}`), scan.Hooks["H"])
		assert.Equal(t, []string{"A"}, scan.Permissions.Allow)
		assert.Equal(t, []string{"D"}, scan.Permissions.Deny)
		assert.Equal(t, json.RawMessage(`{}`), scan.MCP["M"])
		assert.Equal(t, "bind", scan.Keybindings["B"])

		// All items should be marked as config-only.
		assert.True(t, scan.ConfigOnly["p1@mkt"])
		assert.True(t, scan.ConfigOnly[forkedKey])
		assert.True(t, scan.ConfigOnly["k"])
		assert.True(t, scan.ConfigOnly["H"])
		assert.True(t, scan.ConfigOnly["allow:A"])
		assert.True(t, scan.ConfigOnly["deny:D"])
		assert.True(t, scan.ConfigOnly["M"])
		assert.True(t, scan.ConfigOnly["B"])
	})
}

func TestMergeExistingConfig_EmptyConfig(t *testing.T) {
	t.Run("no changes when config is empty", func(t *testing.T) {
		scan := &commands.InitScanResult{
			PluginKeys: []string{"existing@mkt"},
			Upstream:   []string{"existing@mkt"},
			Settings:   map[string]any{"k": "v"},
		}
		cfg := &config.Config{}

		commands.MergeExistingConfig(scan, cfg, "")

		assert.Equal(t, []string{"existing@mkt"}, scan.PluginKeys)
		assert.Equal(t, []string{"existing@mkt"}, scan.Upstream)
		assert.Equal(t, map[string]any{"k": "v"}, scan.Settings)
		assert.Empty(t, scan.ConfigOnly)
	})
}
