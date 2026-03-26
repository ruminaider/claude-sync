package commands_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/ruminaider/claude-sync/internal/claudemd"
	"github.com/ruminaider/claude-sync/internal/cmdskill"
	"github.com/ruminaider/claude-sync/internal/commands"
	"github.com/ruminaider/claude-sync/internal/config"
	plugins "github.com/ruminaider/claude-sync/internal/plugins"
	"github.com/ruminaider/claude-sync/internal/profiles"
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
		assert.True(t, scan.ConfigOnly["plugin:new-one@mkt"])
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
		assert.True(t, scan.ConfigOnly["plugin:"+expectedKey])
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
		assert.True(t, scan.ConfigOnly["setting:newKey"])
		assert.False(t, scan.ConfigOnly["setting:existing"])
	})

	t.Run("initializes nil settings map", func(t *testing.T) {
		scan := &commands.InitScanResult{}
		cfg := &config.Config{
			Settings: map[string]any{"theme": "dark"},
		}

		commands.MergeExistingConfig(scan, cfg, "")

		require.NotNil(t, scan.Settings)
		assert.Equal(t, "dark", scan.Settings["theme"])
		assert.True(t, scan.ConfigOnly["setting:theme"])
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
		assert.True(t, scan.ConfigOnly["hook:PostToolUse"])
		assert.False(t, scan.ConfigOnly["hook:PreToolUse"])
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
		assert.True(t, scan.ConfigOnly["hook:SessionEnd"])
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
		assert.True(t, scan.ConfigOnly["mcp:server-b"])
		assert.False(t, scan.ConfigOnly["mcp:server-a"])
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
		assert.True(t, scan.ConfigOnly["mcp:my-mcp"])
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
		assert.True(t, scan.ConfigOnly["keybinding:ctrl+q"])
		assert.False(t, scan.ConfigOnly["keybinding:ctrl+s"])
	})

	t.Run("initializes nil keybindings map", func(t *testing.T) {
		scan := &commands.InitScanResult{}
		cfg := &config.Config{
			Keybindings: map[string]any{"ctrl+z": "undo"},
		}

		commands.MergeExistingConfig(scan, cfg, "")

		require.NotNil(t, scan.Keybindings)
		assert.Equal(t, "undo", scan.Keybindings["ctrl+z"])
		assert.True(t, scan.ConfigOnly["keybinding:ctrl+z"])
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
		assert.True(t, scan.ConfigOnly["plugin:plugin@mkt"])
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
		assert.True(t, scan.ConfigOnly["setting:newSetting"])
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
		assert.True(t, scan.ConfigOnly["plugin:remote@mkt"])
		assert.True(t, scan.ConfigOnly["setting:remoteSetting"])
		assert.True(t, scan.ConfigOnly["hook:SessionEnd"])
		assert.True(t, scan.ConfigOnly["allow:Bash(npm *)"])
		assert.True(t, scan.ConfigOnly["mcp:remote-mcp"])
		assert.True(t, scan.ConfigOnly["keybinding:ctrl+q"])

		// Pre-existing items should NOT be in ConfigOnly.
		assert.False(t, scan.ConfigOnly["plugin:local@mkt"])
		assert.False(t, scan.ConfigOnly["setting:localSetting"])
		assert.False(t, scan.ConfigOnly["hook:PreToolUse"])
		assert.False(t, scan.ConfigOnly["allow:Bash(git *)"])
		assert.False(t, scan.ConfigOnly["mcp:local-mcp"])
		assert.False(t, scan.ConfigOnly["keybinding:ctrl+s"])
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
		assert.True(t, scan.ConfigOnly["plugin:p1@mkt"])
		assert.True(t, scan.ConfigOnly["plugin:"+forkedKey])
		assert.True(t, scan.ConfigOnly["setting:k"])
		assert.True(t, scan.ConfigOnly["hook:H"])
		assert.True(t, scan.ConfigOnly["allow:A"])
		assert.True(t, scan.ConfigOnly["deny:D"])
		assert.True(t, scan.ConfigOnly["mcp:M"])
		assert.True(t, scan.ConfigOnly["keybinding:B"])
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

func TestMergeExistingConfig_ClaudeMDFragments(t *testing.T) {
	t.Run("injects missing fragment from sync dir", func(t *testing.T) {
		syncDir := t.TempDir()
		claudeMdDir := filepath.Join(syncDir, "claude-md")
		require.NoError(t, os.MkdirAll(claudeMdDir, 0755))

		// Write a fragment file that the config references but the scan doesn't have.
		fragContent := "## Coding Standards\n\nUse gofmt. No globals."
		require.NoError(t, os.WriteFile(
			filepath.Join(claudeMdDir, "coding-standards.md"),
			[]byte(fragContent), 0644,
		))

		// The scan already has one section; config references both.
		scan := &commands.InitScanResult{
			ClaudeMDSections: []claudemd.Section{
				{Header: "Existing Section", Content: "## Existing Section\n\nAlready here."},
			},
		}
		cfg := &config.Config{
			ClaudeMD: config.ClaudeMDConfig{
				Include: []string{"existing-section", "coding-standards"},
			},
		}

		commands.MergeExistingConfig(scan, cfg, syncDir)

		// Should not duplicate the existing section, and should inject the missing one.
		require.Len(t, scan.ClaudeMDSections, 2)
		assert.Equal(t, "Existing Section", scan.ClaudeMDSections[0].Header)
		assert.Equal(t, "Coding Standards", scan.ClaudeMDSections[1].Header)
		assert.Contains(t, scan.ClaudeMDSections[1].Content, "Use gofmt")

		// Only the injected fragment should be marked config-only.
		assert.True(t, scan.ConfigOnly["fragment:coding-standards"])
		assert.False(t, scan.ConfigOnly["fragment:existing-section"])
	})
}

func TestMergeExistingConfig_ClaudeMDFragments_MissingFile(t *testing.T) {
	t.Run("injects placeholder when fragment file does not exist", func(t *testing.T) {
		syncDir := t.TempDir()
		// Don't create any fragment files; the claude-md dir doesn't even exist.

		scan := &commands.InitScanResult{}
		cfg := &config.Config{
			ClaudeMD: config.ClaudeMDConfig{
				Include: []string{"nonexistent-fragment"},
			},
		}

		commands.MergeExistingConfig(scan, cfg, syncDir)

		require.Len(t, scan.ClaudeMDSections, 1)
		assert.Equal(t, "nonexistent-fragment", scan.ClaudeMDSections[0].Header)
		assert.Contains(t, scan.ClaudeMDSections[0].Content, "not available locally")
		assert.True(t, scan.ConfigOnly["fragment:nonexistent-fragment"])
	})
}

func TestMergeExistingConfig_ClaudeMDFragments_NoDuplicates(t *testing.T) {
	t.Run("does not duplicate fragments already in scan", func(t *testing.T) {
		syncDir := t.TempDir()
		claudeMdDir := filepath.Join(syncDir, "claude-md")
		require.NoError(t, os.MkdirAll(claudeMdDir, 0755))

		// Write the fragment file (it exists in the sync dir).
		require.NoError(t, os.WriteFile(
			filepath.Join(claudeMdDir, "coding-standards.md"),
			[]byte("## Coding Standards\n\nContent."), 0644,
		))

		// The scan already contains both fragments the config references.
		scan := &commands.InitScanResult{
			ClaudeMDSections: []claudemd.Section{
				{Header: "Coding Standards", Content: "## Coding Standards\n\nContent."},
				{Header: "Testing Guide", Content: "## Testing Guide\n\nTest everything."},
			},
		}
		cfg := &config.Config{
			ClaudeMD: config.ClaudeMDConfig{
				Include: []string{"coding-standards", "testing-guide"},
			},
		}

		commands.MergeExistingConfig(scan, cfg, syncDir)

		// No new sections should be added.
		assert.Len(t, scan.ClaudeMDSections, 2)
		// Nothing should be marked config-only for these fragments.
		assert.False(t, scan.ConfigOnly["fragment:coding-standards"])
		assert.False(t, scan.ConfigOnly["fragment:testing-guide"])
	})
}

func TestMergeClaudeMDFragments_SkipsUnavailableProjectFragment(t *testing.T) {
	t.Run("does not inject placeholder for unavailable project fragment", func(t *testing.T) {
		syncDir := t.TempDir()
		// No exported project fragment file exists in claude-md/.

		scan := &commands.InitScanResult{}
		cfg := &config.Config{
			ClaudeMD: config.ClaudeMDConfig{
				Include: []string{"~/Work/evvy/CLAUDE.md::beads-issue-tracking"},
			},
		}

		commands.MergeExistingConfig(scan, cfg, syncDir)

		// No section should be added: project fragments without exported
		// content are silently skipped to avoid garbled qualified keys.
		assert.Empty(t, scan.ClaudeMDSections)

		// The key should still be tracked in ConfigOnly so it isn't lost.
		assert.True(t, scan.ConfigOnly["fragment:~/Work/evvy/CLAUDE.md::beads-issue-tracking"])
	})
}

func TestMergeClaudeMDFragments_InjectsAvailableProjectFragment(t *testing.T) {
	t.Run("injects sections with Source from exported project fragment", func(t *testing.T) {
		syncDir := t.TempDir()
		claudeMdDir := filepath.Join(syncDir, "claude-md")
		require.NoError(t, os.MkdirAll(claudeMdDir, 0755))

		qualifiedKey := "~/Work/evvy/CLAUDE.md::beads-issue-tracking"
		fragFilename := claudemd.ProjectFragmentFilename(qualifiedKey)
		fragContent := "## Beads Issue Tracking\n\nTrack issues in Linear project BEADS."

		require.NoError(t, os.WriteFile(
			filepath.Join(claudeMdDir, fragFilename+".md"),
			[]byte(fragContent), 0644,
		))

		scan := &commands.InitScanResult{}
		cfg := &config.Config{
			ClaudeMD: config.ClaudeMDConfig{
				Include: []string{qualifiedKey},
			},
		}

		commands.MergeExistingConfig(scan, cfg, syncDir)

		// The exported content should be split and injected with the correct Source.
		require.Len(t, scan.ClaudeMDSections, 1)
		assert.Equal(t, "Beads Issue Tracking", scan.ClaudeMDSections[0].Header)
		assert.Equal(t, "~/Work/evvy/CLAUDE.md", scan.ClaudeMDSections[0].Source)
		assert.Contains(t, scan.ClaudeMDSections[0].Content, "Track issues in Linear project BEADS")
		assert.True(t, scan.ConfigOnly["fragment:"+qualifiedKey])
	})
}

func TestMergeExistingConfig_ClaudeMDFragments_EmptyInclude(t *testing.T) {
	t.Run("no-op when config has no CLAUDE.md includes", func(t *testing.T) {
		scan := &commands.InitScanResult{}
		cfg := &config.Config{}

		commands.MergeExistingConfig(scan, cfg, "")

		assert.Nil(t, scan.ClaudeMDSections)
	})
}

func TestMergeExistingConfig_Commands(t *testing.T) {
	t.Run("injects missing command with content from sync dir", func(t *testing.T) {
		syncDir := t.TempDir()
		commandsDir := filepath.Join(syncDir, "commands")
		require.NoError(t, os.MkdirAll(commandsDir, 0755))

		cmdContent := "---\ndescription: Review a plan\n---\nReview the plan carefully."
		require.NoError(t, os.WriteFile(
			filepath.Join(commandsDir, "review-plan.md"),
			[]byte(cmdContent), 0644,
		))

		// Scan already has one command; config references both.
		existingItem := cmdskill.Item{
			Name:        "deploy",
			Type:        cmdskill.TypeCommand,
			Source:      cmdskill.SourceGlobal,
			SourceLabel: "global",
			Content:     "Deploy the application.",
		}
		scan := &commands.InitScanResult{
			CommandsSkills: &cmdskill.ScanResult{
				Items: []cmdskill.Item{existingItem},
			},
		}
		cfg := &config.Config{
			Commands: []string{"cmd:global:deploy", "cmd:global:review-plan"},
		}

		commands.MergeExistingConfig(scan, cfg, syncDir)

		require.Len(t, scan.CommandsSkills.Items, 2)

		// The existing item should be untouched.
		assert.Equal(t, "deploy", scan.CommandsSkills.Items[0].Name)

		// The injected item should have content from the sync dir.
		injected := scan.CommandsSkills.Items[1]
		assert.Equal(t, "review-plan", injected.Name)
		assert.Equal(t, cmdskill.TypeCommand, injected.Type)
		assert.Equal(t, cmdskill.SourceGlobal, injected.Source)
		assert.Equal(t, "global", injected.SourceLabel)
		assert.Equal(t, cmdContent, injected.Content)

		// Only the injected command should be marked config-only.
		assert.True(t, scan.ConfigOnly["cmd:global:review-plan"])
		assert.False(t, scan.ConfigOnly["cmd:global:deploy"])
	})
}

func TestMergeExistingConfig_Skills(t *testing.T) {
	t.Run("injects missing skill with content from sync dir", func(t *testing.T) {
		syncDir := t.TempDir()
		skillDir := filepath.Join(syncDir, "skills", "termdock-ast")
		require.NoError(t, os.MkdirAll(skillDir, 0755))

		skillContent := "---\ndescription: AST analysis skill\n---\nAnalyze AST structures."
		require.NoError(t, os.WriteFile(
			filepath.Join(skillDir, "SKILL.md"),
			[]byte(skillContent), 0644,
		))

		scan := &commands.InitScanResult{}
		cfg := &config.Config{
			Skills: []string{"skill:global:termdock-ast"},
		}

		commands.MergeExistingConfig(scan, cfg, syncDir)

		require.NotNil(t, scan.CommandsSkills)
		require.Len(t, scan.CommandsSkills.Items, 1)

		injected := scan.CommandsSkills.Items[0]
		assert.Equal(t, "termdock-ast", injected.Name)
		assert.Equal(t, cmdskill.TypeSkill, injected.Type)
		assert.Equal(t, cmdskill.SourceGlobal, injected.Source)
		assert.Equal(t, "global", injected.SourceLabel)
		assert.Equal(t, skillContent, injected.Content)

		assert.True(t, scan.ConfigOnly["skill:global:termdock-ast"])
	})
}

func TestMergeExistingConfig_CommandsSkills_NoDuplicates(t *testing.T) {
	t.Run("does not duplicate commands already in scan", func(t *testing.T) {
		syncDir := t.TempDir()

		existingItem := cmdskill.Item{
			Name:        "review-plan",
			Type:        cmdskill.TypeCommand,
			Source:      cmdskill.SourceGlobal,
			SourceLabel: "global",
			Content:     "Review the plan.",
		}
		scan := &commands.InitScanResult{
			CommandsSkills: &cmdskill.ScanResult{
				Items: []cmdskill.Item{existingItem},
			},
		}
		cfg := &config.Config{
			Commands: []string{"cmd:global:review-plan"},
		}

		commands.MergeExistingConfig(scan, cfg, syncDir)

		// Should still have exactly one item.
		assert.Len(t, scan.CommandsSkills.Items, 1)
		assert.Equal(t, "review-plan", scan.CommandsSkills.Items[0].Name)

		// Should not be marked config-only since it was already in the scan.
		assert.False(t, scan.ConfigOnly["cmd:global:review-plan"])
	})

	t.Run("does not duplicate skills already in scan", func(t *testing.T) {
		syncDir := t.TempDir()

		existingItem := cmdskill.Item{
			Name:        "brainstorming",
			Type:        cmdskill.TypeSkill,
			Source:      cmdskill.SourceGlobal,
			SourceLabel: "global",
			Content:     "Brainstorm ideas.",
		}
		scan := &commands.InitScanResult{
			CommandsSkills: &cmdskill.ScanResult{
				Items: []cmdskill.Item{existingItem},
			},
		}
		cfg := &config.Config{
			Skills: []string{"skill:global:brainstorming"},
		}

		commands.MergeExistingConfig(scan, cfg, syncDir)

		assert.Len(t, scan.CommandsSkills.Items, 1)
		assert.False(t, scan.ConfigOnly["skill:global:brainstorming"])
	})
}

func TestMergeExistingConfig_CommandsSkills_MissingFile(t *testing.T) {
	t.Run("injects placeholder when command file does not exist", func(t *testing.T) {
		syncDir := t.TempDir()
		// Don't create any command files.

		scan := &commands.InitScanResult{}
		cfg := &config.Config{
			Commands: []string{"cmd:global:nonexistent-cmd"},
		}

		commands.MergeExistingConfig(scan, cfg, syncDir)

		require.NotNil(t, scan.CommandsSkills)
		require.Len(t, scan.CommandsSkills.Items, 1)

		injected := scan.CommandsSkills.Items[0]
		assert.Equal(t, "nonexistent-cmd", injected.Name)
		assert.Equal(t, cmdskill.TypeCommand, injected.Type)
		assert.Equal(t, "(content not available locally)", injected.Content)
		assert.True(t, scan.ConfigOnly["cmd:global:nonexistent-cmd"])
	})

	t.Run("injects placeholder when skill file does not exist", func(t *testing.T) {
		syncDir := t.TempDir()

		scan := &commands.InitScanResult{}
		cfg := &config.Config{
			Skills: []string{"skill:global:nonexistent-skill"},
		}

		commands.MergeExistingConfig(scan, cfg, syncDir)

		require.NotNil(t, scan.CommandsSkills)
		require.Len(t, scan.CommandsSkills.Items, 1)

		injected := scan.CommandsSkills.Items[0]
		assert.Equal(t, "nonexistent-skill", injected.Name)
		assert.Equal(t, cmdskill.TypeSkill, injected.Type)
		assert.Equal(t, "(content not available locally)", injected.Content)
		assert.True(t, scan.ConfigOnly["skill:global:nonexistent-skill"])
	})
}

func TestMergeExistingConfig_CommandsSkills_EmptyConfig(t *testing.T) {
	t.Run("no-op when config has no commands or skills", func(t *testing.T) {
		scan := &commands.InitScanResult{}
		cfg := &config.Config{}

		commands.MergeExistingConfig(scan, cfg, "")

		assert.Nil(t, scan.CommandsSkills)
	})
}

func TestMergeExistingConfig_CommandsSkills_MixedTypes(t *testing.T) {
	t.Run("injects both commands and skills together", func(t *testing.T) {
		syncDir := t.TempDir()

		// Create a command file.
		commandsDir := filepath.Join(syncDir, "commands")
		require.NoError(t, os.MkdirAll(commandsDir, 0755))
		require.NoError(t, os.WriteFile(
			filepath.Join(commandsDir, "my-cmd.md"),
			[]byte("Command content."), 0644,
		))

		// Create a skill file.
		skillDir := filepath.Join(syncDir, "skills", "my-skill")
		require.NoError(t, os.MkdirAll(skillDir, 0755))
		require.NoError(t, os.WriteFile(
			filepath.Join(skillDir, "SKILL.md"),
			[]byte("Skill content."), 0644,
		))

		scan := &commands.InitScanResult{}
		cfg := &config.Config{
			Commands: []string{"cmd:global:my-cmd"},
			Skills:   []string{"skill:global:my-skill"},
		}

		commands.MergeExistingConfig(scan, cfg, syncDir)

		require.NotNil(t, scan.CommandsSkills)
		require.Len(t, scan.CommandsSkills.Items, 2)

		// Verify command.
		assert.Equal(t, "my-cmd", scan.CommandsSkills.Items[0].Name)
		assert.Equal(t, cmdskill.TypeCommand, scan.CommandsSkills.Items[0].Type)
		assert.Equal(t, "Command content.", scan.CommandsSkills.Items[0].Content)

		// Verify skill.
		assert.Equal(t, "my-skill", scan.CommandsSkills.Items[1].Name)
		assert.Equal(t, cmdskill.TypeSkill, scan.CommandsSkills.Items[1].Type)
		assert.Equal(t, "Skill content.", scan.CommandsSkills.Items[1].Content)

		assert.True(t, scan.ConfigOnly["cmd:global:my-cmd"])
		assert.True(t, scan.ConfigOnly["skill:global:my-skill"])
	})
}

func TestMergeExistingConfig_CommandsSkills_SkipsNonGlobalScope(t *testing.T) {
	t.Run("skips plugin-scoped and project-scoped keys", func(t *testing.T) {
		syncDir := t.TempDir()

		scan := &commands.InitScanResult{}
		cfg := &config.Config{
			Commands: []string{"cmd:plugin:myplugin:some-cmd", "cmd:project:myproj:deploy"},
		}

		commands.MergeExistingConfig(scan, cfg, syncDir)

		// ScanResult is initialized but no items should be injected for non-global scope.
		require.NotNil(t, scan.CommandsSkills)
		assert.Empty(t, scan.CommandsSkills.Items)
	})
}

func TestMergeExistingConfig_CommandsSkills_InvalidKeys(t *testing.T) {
	t.Run("skips malformed keys", func(t *testing.T) {
		syncDir := t.TempDir()

		scan := &commands.InitScanResult{}
		cfg := &config.Config{
			Commands: []string{"invalid", "also:invalid"},
		}

		commands.MergeExistingConfig(scan, cfg, syncDir)

		// ScanResult should be initialized but empty (since we had non-empty allKeys).
		require.NotNil(t, scan.CommandsSkills)
		assert.Empty(t, scan.CommandsSkills.Items)
	})
}

// ---------------------------------------------------------------------------
// mergeMemory helper tests
// ---------------------------------------------------------------------------

func TestMergeMemory_InjectsNew(t *testing.T) {
	scan := &commands.InitScanResult{
		MemoryFiles: []string{"existing-mem"},
		ConfigOnly:  make(map[string]bool),
	}
	cfg := &config.Config{
		Memory: config.MemoryConfig{
			Include: []string{"existing-mem", "new-mem"},
		},
	}

	commands.MergeExisting(scan, cfg, nil, "")

	assert.Contains(t, scan.MemoryFiles, "new-mem")
	assert.True(t, scan.ConfigOnly["memory:new-mem"])
}

func TestMergeMemory_SkipsExisting(t *testing.T) {
	scan := &commands.InitScanResult{
		MemoryFiles: []string{"already-here"},
		ConfigOnly:  make(map[string]bool),
	}
	cfg := &config.Config{
		Memory: config.MemoryConfig{
			Include: []string{"already-here"},
		},
	}

	commands.MergeExisting(scan, cfg, nil, "")

	assert.Equal(t, []string{"already-here"}, scan.MemoryFiles)
	assert.Empty(t, scan.ConfigOnly)
}

func TestMergeMemory_NilSlice(t *testing.T) {
	scan := &commands.InitScanResult{
		ConfigOnly: make(map[string]bool),
	}
	cfg := &config.Config{
		Memory: config.MemoryConfig{
			Include: []string{"new-mem"},
		},
	}

	commands.MergeExisting(scan, cfg, nil, "")

	assert.Equal(t, []string{"new-mem"}, scan.MemoryFiles)
	assert.True(t, scan.ConfigOnly["memory:new-mem"])
}

// ---------------------------------------------------------------------------
// Combined MergeExisting tests
// ---------------------------------------------------------------------------

func TestMergeExisting_BaseConfigOnly(t *testing.T) {
	scan := &commands.InitScanResult{}
	cfg := &config.Config{
		Upstream:    []string{"p1@mkt"},
		Settings:    map[string]any{"theme": "dark"},
		MCP:         map[string]json.RawMessage{"srv": json.RawMessage(`{"cmd":"x"}`)},
		Permissions: config.Permissions{Allow: []string{"Bash(git *)"}},
	}

	commands.MergeExisting(scan, cfg, nil, "")

	assert.Contains(t, scan.PluginKeys, "p1@mkt")
	assert.Contains(t, scan.Upstream, "p1@mkt")
	assert.Equal(t, "dark", scan.Settings["theme"])
	assert.Equal(t, json.RawMessage(`{"cmd":"x"}`), scan.MCP["srv"])
	assert.Contains(t, scan.Permissions.Allow, "Bash(git *)")

	assert.True(t, scan.ConfigOnly["plugin:p1@mkt"])
	assert.True(t, scan.ConfigOnly["setting:theme"])
	assert.True(t, scan.ConfigOnly["mcp:srv"])
	assert.True(t, scan.ConfigOnly["allow:Bash(git *)"])
}

func TestMergeExisting_ProfilesOnly(t *testing.T) {
	scan := &commands.InitScanResult{}
	profs := map[string]profiles.Profile{
		"dev": {
			Plugins:  profiles.ProfilePlugins{Add: []string{"plugin-a@mkt"}},
			Settings: map[string]any{"editor": "vim"},
			MCP: profiles.ProfileMCP{
				Add: map[string]json.RawMessage{"dev-srv": json.RawMessage(`{"cmd":"dev"}`)},
			},
			Permissions: profiles.ProfilePermissions{AddAllow: []string{"Bash(npm *)"}},
			Hooks: profiles.ProfileHooks{
				Add: map[string]json.RawMessage{"PreToolUse": json.RawMessage(`[{"type":"command","command":"echo"}]`)},
			},
			Keybindings: profiles.ProfileKeybindings{Override: map[string]any{"ctrl+d": "debug"}},
			Memory:      profiles.ProfileMemory{Add: []string{"dev-notes"}},
		},
	}

	commands.MergeExisting(scan, nil, profs, "")

	// Plugins
	assert.Contains(t, scan.PluginKeys, "plugin-a@mkt")
	assert.Contains(t, scan.Upstream, "plugin-a@mkt")
	assert.True(t, scan.ConfigOnly["plugin:plugin-a@mkt"])

	// Settings
	assert.Equal(t, "vim", scan.Settings["editor"])
	assert.True(t, scan.ConfigOnly["setting:editor"])

	// MCP
	assert.Equal(t, json.RawMessage(`{"cmd":"dev"}`), scan.MCP["dev-srv"])
	assert.True(t, scan.ConfigOnly["mcp:dev-srv"])

	// Permissions
	assert.Contains(t, scan.Permissions.Allow, "Bash(npm *)")
	assert.True(t, scan.ConfigOnly["allow:Bash(npm *)"])

	// Hooks
	assert.NotNil(t, scan.Hooks["PreToolUse"])
	assert.True(t, scan.ConfigOnly["hook:PreToolUse"])

	// Keybindings
	assert.Equal(t, "debug", scan.Keybindings["ctrl+d"])
	assert.True(t, scan.ConfigOnly["keybinding:ctrl+d"])

	// Memory
	assert.Contains(t, scan.MemoryFiles, "dev-notes")
	assert.True(t, scan.ConfigOnly["memory:dev-notes"])
}

func TestMergeExisting_BaseAndProfiles(t *testing.T) {
	scan := &commands.InitScanResult{}
	cfg := &config.Config{
		Upstream: []string{"base-plugin@mkt"},
		Settings: map[string]any{"theme": "dark"},
	}
	profs := map[string]profiles.Profile{
		"dev": {
			Plugins:  profiles.ProfilePlugins{Add: []string{"base-plugin@mkt", "dev-plugin@mkt"}},
			Settings: map[string]any{"theme": "light", "editor": "vim"},
		},
	}

	commands.MergeExisting(scan, cfg, profs, "")

	// base-plugin should appear once (base injected it; profile dedup skips it).
	count := 0
	for _, k := range scan.PluginKeys {
		if k == "base-plugin@mkt" {
			count++
		}
	}
	assert.Equal(t, 1, count, "base-plugin@mkt should appear exactly once")

	// dev-plugin should be injected by the profile.
	assert.Contains(t, scan.PluginKeys, "dev-plugin@mkt")
	assert.True(t, scan.ConfigOnly["plugin:dev-plugin@mkt"])

	// "theme" was injected by base config; profile dedup skips it.
	assert.Equal(t, "dark", scan.Settings["theme"])
	assert.True(t, scan.ConfigOnly["setting:theme"])

	// "editor" is new from the profile.
	assert.Equal(t, "vim", scan.Settings["editor"])
	assert.True(t, scan.ConfigOnly["setting:editor"])
}

func TestMergeExisting_MultiProfileDedup(t *testing.T) {
	scan := &commands.InitScanResult{}
	profs := map[string]profiles.Profile{
		"alpha": {
			Plugins:  profiles.ProfilePlugins{Add: []string{"shared@mkt"}},
			Settings: map[string]any{"shared-key": "alpha-val"},
			MCP: profiles.ProfileMCP{
				Add: map[string]json.RawMessage{"shared-mcp": json.RawMessage(`{"from":"alpha"}`)},
			},
		},
		"beta": {
			Plugins:  profiles.ProfilePlugins{Add: []string{"shared@mkt"}},
			Settings: map[string]any{"shared-key": "beta-val"},
			MCP: profiles.ProfileMCP{
				Add: map[string]json.RawMessage{"shared-mcp": json.RawMessage(`{"from":"beta"}`)},
			},
		},
	}

	commands.MergeExisting(scan, nil, profs, "")

	// Plugin should appear exactly once.
	pluginCount := 0
	for _, k := range scan.PluginKeys {
		if k == "shared@mkt" {
			pluginCount++
		}
	}
	assert.Equal(t, 1, pluginCount, "shared@mkt should appear exactly once in PluginKeys")

	// Settings: first profile to be processed wins (map iteration order is
	// nondeterministic, but the value should be one of the two).
	val, ok := scan.Settings["shared-key"]
	assert.True(t, ok)
	assert.Contains(t, []any{"alpha-val", "beta-val"}, val)

	// MCP: same, first wins.
	_, mcpOk := scan.MCP["shared-mcp"]
	assert.True(t, mcpOk)
}

func TestMergeExisting_ForkedProfilePlugin(t *testing.T) {
	scan := &commands.InitScanResult{}
	forkedKey := "my-fork@" + plugins.MarketplaceName
	profs := map[string]profiles.Profile{
		"dev": {
			Plugins: profiles.ProfilePlugins{Add: []string{forkedKey, "normal@mkt"}},
		},
	}

	commands.MergeExisting(scan, nil, profs, "")

	// The forked plugin should end up in AutoForked (with marketplace suffix).
	assert.Contains(t, scan.AutoForked, forkedKey)
	assert.True(t, scan.ConfigOnly["plugin:"+forkedKey])

	// The normal plugin should be in Upstream.
	assert.Contains(t, scan.Upstream, "normal@mkt")
	assert.True(t, scan.ConfigOnly["plugin:normal@mkt"])

	// Both should be in PluginKeys.
	assert.Contains(t, scan.PluginKeys, forkedKey)
	assert.Contains(t, scan.PluginKeys, "normal@mkt")
}

func TestMergeExisting_ConflictBaseVsProfile(t *testing.T) {
	scan := &commands.InitScanResult{}
	cfg := &config.Config{
		MCP: map[string]json.RawMessage{
			"shared-mcp": json.RawMessage(`{"from":"base"}`),
		},
		Settings: map[string]any{"shared-key": "base-val"},
	}
	profs := map[string]profiles.Profile{
		"dev": {
			MCP: profiles.ProfileMCP{
				Add: map[string]json.RawMessage{
					"shared-mcp": json.RawMessage(`{"from":"profile"}`),
				},
			},
			Settings: map[string]any{"shared-key": "profile-val"},
		},
	}

	commands.MergeExisting(scan, cfg, profs, "")

	// Base config was processed first, so its value should be in the scan.
	// Profile dedup sees the key already exists and skips it.
	assert.Equal(t, json.RawMessage(`{"from":"base"}`), scan.MCP["shared-mcp"])
	assert.Equal(t, "base-val", scan.Settings["shared-key"])
}

func TestMergeExisting_EmptyScan(t *testing.T) {
	scan := &commands.InitScanResult{}
	cfg := &config.Config{
		Upstream:    []string{"base@mkt"},
		Forked:      []string{"fork1"},
		Settings:    map[string]any{"k": "v"},
		Hooks:       map[string]json.RawMessage{"H": json.RawMessage(`{}`)},
		Permissions: config.Permissions{Allow: []string{"A"}, Deny: []string{"D"}},
		MCP:         map[string]json.RawMessage{"M": json.RawMessage(`{}`)},
		Keybindings: map[string]any{"B": "bind"},
		Memory:      config.MemoryConfig{Include: []string{"mem1"}},
	}
	profs := map[string]profiles.Profile{
		"extra": {
			Plugins:     profiles.ProfilePlugins{Add: []string{"prof-plugin@mkt"}},
			Settings:    map[string]any{"pk": "pv"},
			Permissions: profiles.ProfilePermissions{AddAllow: []string{"PA"}, AddDeny: []string{"PD"}},
			Memory:      profiles.ProfileMemory{Add: []string{"prof-mem"}},
		},
	}

	commands.MergeExisting(scan, cfg, profs, "")

	forkedKey := "fork1@" + plugins.MarketplaceName

	// Base items.
	assert.Contains(t, scan.PluginKeys, "base@mkt")
	assert.Contains(t, scan.PluginKeys, forkedKey)
	assert.Equal(t, "v", scan.Settings["k"])
	assert.NotNil(t, scan.Hooks["H"])
	assert.Contains(t, scan.Permissions.Allow, "A")
	assert.Contains(t, scan.Permissions.Deny, "D")
	assert.NotNil(t, scan.MCP["M"])
	assert.Equal(t, "bind", scan.Keybindings["B"])
	assert.Contains(t, scan.MemoryFiles, "mem1")

	// Profile items.
	assert.Contains(t, scan.PluginKeys, "prof-plugin@mkt")
	assert.Equal(t, "pv", scan.Settings["pk"])
	assert.Contains(t, scan.Permissions.Allow, "PA")
	assert.Contains(t, scan.Permissions.Deny, "PD")
	assert.Contains(t, scan.MemoryFiles, "prof-mem")

	// ConfigOnly tracking for all injected items.
	assert.True(t, scan.ConfigOnly["plugin:base@mkt"])
	assert.True(t, scan.ConfigOnly["plugin:"+forkedKey])
	assert.True(t, scan.ConfigOnly["setting:k"])
	assert.True(t, scan.ConfigOnly["hook:H"])
	assert.True(t, scan.ConfigOnly["allow:A"])
	assert.True(t, scan.ConfigOnly["deny:D"])
	assert.True(t, scan.ConfigOnly["mcp:M"])
	assert.True(t, scan.ConfigOnly["keybinding:B"])
	assert.True(t, scan.ConfigOnly["memory:mem1"])
	assert.True(t, scan.ConfigOnly["plugin:prof-plugin@mkt"])
	assert.True(t, scan.ConfigOnly["setting:pk"])
	assert.True(t, scan.ConfigOnly["allow:PA"])
	assert.True(t, scan.ConfigOnly["deny:PD"])
	assert.True(t, scan.ConfigOnly["memory:prof-mem"])
}
