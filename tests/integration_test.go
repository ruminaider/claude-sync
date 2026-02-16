//go:build integration

package tests

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"testing"

	"github.com/ruminaider/claude-sync/internal/approval"
	"github.com/ruminaider/claude-sync/internal/claudecode"
	"github.com/ruminaider/claude-sync/internal/commands"
	"github.com/ruminaider/claude-sync/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupMockClaude creates a minimal Claude Code directory with the given plugins.
// It writes plugins/installed_plugins.json (v2 format), plugins/known_marketplaces.json,
// and settings.json.
func setupMockClaude(t *testing.T, dir string, plugins []string) {
	t.Helper()

	pluginDir := filepath.Join(dir, "plugins")
	require.NoError(t, os.MkdirAll(pluginDir, 0755))

	pluginsMap := make(map[string][]claudecode.PluginInstallation)
	for _, p := range plugins {
		pluginsMap[p] = []claudecode.PluginInstallation{
			{
				Scope:       "user",
				InstallPath: "/mock/" + p,
				Version:     "1.0.0",
				InstalledAt: "2026-01-01T00:00:00Z",
				LastUpdated: "2026-01-01T00:00:00Z",
			},
		}
	}

	installed := claudecode.InstalledPlugins{
		Version: 2,
		Plugins: pluginsMap,
	}

	data, err := json.MarshalIndent(installed, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(pluginDir, "installed_plugins.json"), data, 0644))

	require.NoError(t, os.WriteFile(filepath.Join(pluginDir, "known_marketplaces.json"), []byte("{}"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "settings.json"), []byte("{}"), 0644))
}

// addPluginToMockClaude reads the existing installed_plugins.json, adds a new plugin entry,
// and writes it back.
func addPluginToMockClaude(t *testing.T, dir string, plugin string) {
	t.Helper()

	path := filepath.Join(dir, "plugins", "installed_plugins.json")
	data, err := os.ReadFile(path)
	require.NoError(t, err)

	var installed claudecode.InstalledPlugins
	require.NoError(t, json.Unmarshal(data, &installed))

	if installed.Plugins == nil {
		installed.Plugins = make(map[string][]claudecode.PluginInstallation)
	}

	installed.Plugins[plugin] = []claudecode.PluginInstallation{
		{
			Scope:       "user",
			InstallPath: "/mock/" + plugin,
			Version:     "1.0.0",
			InstalledAt: "2026-01-28T00:00:00Z",
			LastUpdated: "2026-01-28T00:00:00Z",
		},
	}

	newData, err := json.MarshalIndent(installed, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, newData, 0644))
}

// gitRun executes a git command in the given directory and fails the test on error.
func gitRun(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "git %v failed: %s", args, string(out))
	return string(out)
}

func TestFullWorkflow(t *testing.T) {
	// Create all directories using t.TempDir() for automatic cleanup.
	machine1Claude := t.TempDir()
	machine1Sync := filepath.Join(t.TempDir(), ".claude-sync")
	machine2Claude := t.TempDir()
	machine2Sync := filepath.Join(t.TempDir(), ".claude-sync")

	// Create a bare "remote" git repo to simulate a shared origin.
	remoteDir := filepath.Join(t.TempDir(), "remote.git")
	gitRun(t, ".", "init", "--bare", remoteDir)

	// ---------------------------------------------------------------
	// Step 1: Machine 1 — set up mock Claude Code dir with two plugins.
	// ---------------------------------------------------------------
	setupMockClaude(t, machine1Claude, []string{
		"context7@claude-plugins-official",
		"beads@beads-marketplace",
	})

	// ---------------------------------------------------------------
	// Step 2: Machine 1 — Init.
	// ---------------------------------------------------------------
	_, err := commands.Init(commands.InitOptions{
		ClaudeDir:       machine1Claude,
		SyncDir:         machine1Sync,
		IncludeSettings: true,
		IncludeHooks:    nil,
	})
	require.NoError(t, err, "Init should succeed on machine 1")

	// ---------------------------------------------------------------
	// Step 3: Machine 1 — Add remote and push.
	// ---------------------------------------------------------------
	gitRun(t, machine1Sync, "remote", "add", "origin", remoteDir)
	gitRun(t, machine1Sync, "push", "-u", "origin", "HEAD")

	// ---------------------------------------------------------------
	// Step 4: Machine 1 — Status should show 2 synced, 0 not installed.
	// ---------------------------------------------------------------
	status1, err := commands.Status(machine1Claude, machine1Sync)
	require.NoError(t, err, "Status should succeed on machine 1")

	assert.Len(t, status1.Synced, 2, "Machine 1 should have 2 synced plugins")
	assert.Empty(t, status1.NotInstalled, "Machine 1 should have 0 not-installed plugins")

	// ---------------------------------------------------------------
	// Step 5: Machine 2 — set up mock Claude dir with only context7.
	// ---------------------------------------------------------------
	setupMockClaude(t, machine2Claude, []string{
		"context7@claude-plugins-official",
	})

	// ---------------------------------------------------------------
	// Step 6: Machine 2 — Join from the remote.
	// ---------------------------------------------------------------
	_, err = commands.Join(remoteDir, machine2Claude, machine2Sync)
	require.NoError(t, err, "Join should succeed on machine 2")

	// ---------------------------------------------------------------
	// Step 7: Machine 2 — Status should show beads in NotInstalled.
	// ---------------------------------------------------------------
	status2, err := commands.Status(machine2Claude, machine2Sync)
	require.NoError(t, err, "Status should succeed on machine 2")

	assert.Contains(t, status2.NotInstalled, "beads@beads-marketplace",
		"Machine 2 should report beads@beads-marketplace as not installed")

	// ---------------------------------------------------------------
	// Step 8: Machine 1 — Add new-plugin@local to mock Claude dir.
	// ---------------------------------------------------------------
	addPluginToMockClaude(t, machine1Claude, "new-plugin@local")

	// ---------------------------------------------------------------
	// Step 9: Machine 1 — PushApply to add the new plugin.
	// ---------------------------------------------------------------
	err = commands.PushApply(commands.PushApplyOptions{
		ClaudeDir:  machine1Claude,
		SyncDir:    machine1Sync,
		AddPlugins: []string{"new-plugin@local"},
		Message:    "Add new plugin",
	})
	require.NoError(t, err, "PushApply should succeed on machine 1")

	// ---------------------------------------------------------------
	// Step 10: Verify config.yaml now contains new-plugin@local.
	// ---------------------------------------------------------------
	cfgData, err := os.ReadFile(filepath.Join(machine1Sync, "config.yaml"))
	require.NoError(t, err, "should be able to read config.yaml after push")

	cfg, err := config.Parse(cfgData)
	require.NoError(t, err, "should be able to parse config.yaml after push")

	sort.Strings(cfg.Upstream)
	assert.Contains(t, cfg.Upstream, "new-plugin@local",
		"config.yaml should contain new-plugin@local after PushApply")
	assert.Contains(t, cfg.Upstream, "context7@claude-plugins-official",
		"config.yaml should still contain context7")
	assert.Contains(t, cfg.Upstream, "beads@beads-marketplace",
		"config.yaml should still contain beads")
	assert.Len(t, cfg.Upstream, 3, "config.yaml should have exactly 3 plugins")
}

func TestV2Workflow(t *testing.T) {
	claudeDir := t.TempDir()
	syncDir := filepath.Join(t.TempDir(), ".claude-sync")

	// ---------------------------------------------------------------
	// Step 1: Set up mock Claude Code dir with 3 plugins.
	// ---------------------------------------------------------------
	setupMockClaude(t, claudeDir, []string{
		"context7@claude-plugins-official",
		"beads@beads-marketplace",
		"my-plugin@my-marketplace",
	})

	// ---------------------------------------------------------------
	// Step 2: Init — should create v2 config with all plugins in Upstream.
	// ---------------------------------------------------------------
	_, err := commands.Init(commands.InitOptions{
		ClaudeDir:       claudeDir,
		SyncDir:         syncDir,
		IncludeSettings: true,
		IncludeHooks:    nil,
	})
	require.NoError(t, err, "Init should succeed")

	cfgData, err := os.ReadFile(filepath.Join(syncDir, "config.yaml"))
	require.NoError(t, err, "should be able to read config.yaml after init")

	cfg, err := config.Parse(cfgData)
	require.NoError(t, err, "should be able to parse config.yaml after init")

	// ---------------------------------------------------------------
	// Step 3: Verify v2 format — version 2.1.0, all 3 plugins in Upstream.
	// ---------------------------------------------------------------
	assert.Equal(t, "1.0.0", cfg.Version, "config version should be 1.0.0")
	assert.Len(t, cfg.Upstream, 3, "all 3 plugins should be in upstream")
	assert.Empty(t, cfg.Pinned, "no plugins should be pinned initially")
	assert.Empty(t, cfg.Forked, "no plugins should be forked initially")

	// ---------------------------------------------------------------
	// Step 4: Pin "beads@beads-marketplace" at version "0.44.0".
	// ---------------------------------------------------------------
	err = commands.Pin(syncDir, "beads@beads-marketplace", "0.44.0")
	require.NoError(t, err, "Pin should succeed")

	cfgData, err = os.ReadFile(filepath.Join(syncDir, "config.yaml"))
	require.NoError(t, err, "should be able to read config.yaml after pin")

	cfg, err = config.Parse(cfgData)
	require.NoError(t, err, "should be able to parse config.yaml after pin")

	// ---------------------------------------------------------------
	// Step 5: Verify pin — upstream has 2, pinned has beads at 0.44.0.
	// ---------------------------------------------------------------
	assert.Len(t, cfg.Upstream, 2, "upstream should have 2 plugins after pin")
	assert.Equal(t, "0.44.0", cfg.Pinned["beads@beads-marketplace"],
		"beads should be pinned at version 0.44.0")

	// ---------------------------------------------------------------
	// Step 6: Status — check categorized fields.
	// ---------------------------------------------------------------
	status, err := commands.Status(claudeDir, syncDir)
	require.NoError(t, err, "Status should succeed")

	assert.Equal(t, "1.0.0", status.ConfigVersion, "status config version should be 1.0.0")
	assert.NotEmpty(t, status.UpstreamSynced, "should have upstream synced plugins")
	assert.NotEmpty(t, status.PinnedSynced, "should have pinned synced plugins")
	assert.Len(t, status.UpstreamSynced, 2, "should have 2 upstream synced plugins")
	assert.Len(t, status.PinnedSynced, 1, "should have 1 pinned synced plugin")
	assert.Equal(t, "beads@beads-marketplace", status.PinnedSynced[0].Key,
		"pinned synced plugin should be beads")

	// ---------------------------------------------------------------
	// Step 7: JSON output — verify it contains expected keys.
	// ---------------------------------------------------------------
	jsonData, err := status.JSON()
	require.NoError(t, err, "JSON() should succeed")

	var jsonMap map[string]interface{}
	require.NoError(t, json.Unmarshal(jsonData, &jsonMap), "JSON output should be valid JSON")
	assert.Contains(t, string(jsonData), "upstream_synced",
		"JSON output should contain upstream_synced key")
	assert.Contains(t, string(jsonData), "pinned_synced",
		"JSON output should contain pinned_synced key")
	assert.Equal(t, "1.0.0", jsonMap["config_version"],
		"JSON config_version should be 1.0.0")

	// ---------------------------------------------------------------
	// Step 8: Unpin "beads@beads-marketplace".
	// ---------------------------------------------------------------
	err = commands.Unpin(syncDir, "beads@beads-marketplace")
	require.NoError(t, err, "Unpin should succeed")

	cfgData, err = os.ReadFile(filepath.Join(syncDir, "config.yaml"))
	require.NoError(t, err, "should be able to read config.yaml after unpin")

	cfg, err = config.Parse(cfgData)
	require.NoError(t, err, "should be able to parse config.yaml after unpin")

	// ---------------------------------------------------------------
	// Step 9: Verify unpin — all 3 back in upstream, pinned is empty.
	// ---------------------------------------------------------------
	assert.Len(t, cfg.Upstream, 3, "upstream should have 3 plugins after unpin")
	assert.Empty(t, cfg.Pinned, "pinned should be empty after unpin")
	assert.Contains(t, cfg.Upstream, "beads@beads-marketplace",
		"beads should be back in upstream after unpin")
}

func TestFullSurfaceSync(t *testing.T) {
	machine1Claude := t.TempDir()
	machine1Sync := filepath.Join(t.TempDir(), ".claude-sync")
	machine2Claude := t.TempDir()
	machine2Sync := filepath.Join(t.TempDir(), ".claude-sync")

	remoteDir := filepath.Join(t.TempDir(), "remote.git")
	gitRun(t, ".", "init", "--bare", remoteDir)

	// ---------------------------------------------------------------
	// Step 1: Machine 1 — set up Claude Code dir with 2 plugins.
	// ---------------------------------------------------------------
	setupMockClaude(t, machine1Claude, []string{
		"context7@claude-plugins-official",
		"beads@beads-marketplace",
	})

	// Write settings with model and permissions into settings.json.
	settingsData, err := json.MarshalIndent(map[string]any{
		"model": "opus",
		"permissions": map[string]any{
			"allow": []string{"Bash(*)"},
			"deny":  []string{"rm -rf"},
		},
	}, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(machine1Claude, "settings.json"), settingsData, 0644))

	// Write CLAUDE.md.
	claudeMDContent := "## Git Conventions\nUse conventional commits.\n\n## Testing\nAlways write tests."
	require.NoError(t, os.WriteFile(filepath.Join(machine1Claude, "CLAUDE.md"), []byte(claudeMDContent), 0644))

	// Write MCP config.
	mcpData, err := json.MarshalIndent(map[string]any{
		"mcpServers": map[string]any{
			"test-server": map[string]any{"command": "test-cmd"},
		},
	}, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(machine1Claude, ".mcp.json"), mcpData, 0644))

	// Write keybindings.
	kbData, err := json.MarshalIndent(map[string]any{
		"submitWithEnter": false,
	}, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(machine1Claude, "keybindings.json"), kbData, 0644))

	// ---------------------------------------------------------------
	// Step 2: Machine 1 — Init with ALL surfaces enabled.
	// ---------------------------------------------------------------
	mcpServers := map[string]json.RawMessage{
		"test-server": json.RawMessage(`{"command":"test-cmd"}`),
	}
	keybindings := map[string]any{
		"submitWithEnter": false,
	}

	initResult, err := commands.Init(commands.InitOptions{
		ClaudeDir:       machine1Claude,
		SyncDir:         machine1Sync,
		IncludeSettings: true,
		Permissions:     config.Permissions{Allow: []string{"Bash(*)"}, Deny: []string{"rm -rf"}},
		ImportClaudeMD:  true,
		MCP:             mcpServers,
		Keybindings:     keybindings,
	})
	require.NoError(t, err, "Init should succeed on machine 1")
	assert.True(t, initResult.PermissionsIncluded, "permissions should be included")
	assert.Contains(t, initResult.MCPIncluded, "test-server", "MCP should include test-server")
	assert.True(t, initResult.KeybindingsIncluded, "keybindings should be included")
	assert.NotEmpty(t, initResult.ClaudeMDFragments, "CLAUDE.md fragments should be created")

	// ---------------------------------------------------------------
	// Step 3: Machine 1 — Add remote and push.
	// ---------------------------------------------------------------
	gitRun(t, machine1Sync, "remote", "add", "origin", remoteDir)
	gitRun(t, machine1Sync, "push", "-u", "origin", "HEAD")

	// ---------------------------------------------------------------
	// Step 4: Machine 2 — set up mock Claude dir with 1 plugin.
	// ---------------------------------------------------------------
	setupMockClaude(t, machine2Claude, []string{
		"context7@claude-plugins-official",
	})

	// ---------------------------------------------------------------
	// Step 5: Machine 2 — Join from the remote.
	// ---------------------------------------------------------------
	_, err = commands.Join(remoteDir, machine2Claude, machine2Sync)
	require.NoError(t, err, "Join should succeed on machine 2")

	// ---------------------------------------------------------------
	// Step 6: Machine 2 — Pull in AUTO mode.
	// ---------------------------------------------------------------
	result, err := commands.PullWithOptions(commands.PullOptions{
		ClaudeDir: machine2Claude,
		SyncDir:   machine2Sync,
		Auto:      true,
	})
	require.NoError(t, err, "PullWithOptions should succeed on machine 2")

	// ---------------------------------------------------------------
	// Step 7: Verify safe surfaces were applied.
	// ---------------------------------------------------------------

	// Plugins: beads should be in ToInstall (not installed on machine 2).
	assert.Contains(t, result.ToInstall, "beads@beads-marketplace",
		"beads should need installation on machine 2")

	// Settings: model should have been applied to settings.json.
	m2Settings, err := claudecode.ReadSettings(machine2Claude)
	require.NoError(t, err, "should read machine 2 settings")
	assert.Contains(t, string(m2Settings["model"]), "opus",
		"model setting should be applied on machine 2")

	// CLAUDE.md: should be assembled on machine 2.
	assert.True(t, result.ClaudeMDAssembled, "CLAUDE.md should be assembled")
	m2ClaudeMD, err := os.ReadFile(filepath.Join(machine2Claude, "CLAUDE.md"))
	require.NoError(t, err, "should read CLAUDE.md on machine 2")
	assert.Contains(t, string(m2ClaudeMD), "Git Conventions",
		"CLAUDE.md should contain Git Conventions")
	assert.Contains(t, string(m2ClaudeMD), "Testing",
		"CLAUDE.md should contain Testing")

	// Keybindings: should be applied.
	assert.True(t, result.KeybindingsApplied, "keybindings should be applied")
	m2KB, err := claudecode.ReadKeybindings(machine2Claude)
	require.NoError(t, err, "should read keybindings on machine 2")
	assert.Equal(t, false, m2KB["submitWithEnter"],
		"keybinding submitWithEnter should be false")

	// ---------------------------------------------------------------
	// Step 8: Verify high-risk surfaces are deferred in auto mode.
	// ---------------------------------------------------------------
	assert.NotEmpty(t, result.PendingHighRisk, "should have pending high-risk changes")

	// Permissions should NOT be in settings.json yet.
	assert.False(t, result.PermissionsApplied,
		"permissions should NOT be applied in auto mode")

	// MCP should NOT be applied yet.
	assert.Empty(t, result.MCPApplied,
		"MCP should NOT be applied in auto mode")

	// Pending file should exist with permissions and MCP.
	pending, err := approval.ReadPending(machine2Sync)
	require.NoError(t, err, "should read pending changes")
	assert.False(t, pending.IsEmpty(), "pending changes should not be empty")
	require.NotNil(t, pending.Permissions, "pending should have permissions")
	assert.Contains(t, pending.Permissions.Allow, "Bash(*)",
		"pending permissions should include Bash(*)")
	assert.Contains(t, pending.Permissions.Deny, "rm -rf",
		"pending permissions should include rm -rf deny")
	assert.NotEmpty(t, pending.MCP, "pending should have MCP changes")
	assert.Contains(t, pending.MCP, "test-server",
		"pending MCP should include test-server")

	// ---------------------------------------------------------------
	// Step 9: Machine 2 — Approve pending changes.
	// ---------------------------------------------------------------
	approveResult, err := commands.Approve(machine2Claude, machine2Sync)
	require.NoError(t, err, "Approve should succeed on machine 2")

	// ---------------------------------------------------------------
	// Step 10: Verify high-risk surfaces are now applied.
	// ---------------------------------------------------------------
	assert.True(t, approveResult.PermissionsApplied,
		"permissions should be applied after approve")
	assert.Contains(t, approveResult.MCPApplied, "test-server",
		"MCP test-server should be applied after approve")

	// Verify permissions are now in settings.json.
	m2Settings, err = claudecode.ReadSettings(machine2Claude)
	require.NoError(t, err, "should read settings after approve")
	var perms struct {
		Allow []string `json:"allow"`
		Deny  []string `json:"deny"`
	}
	require.NoError(t, json.Unmarshal(m2Settings["permissions"], &perms),
		"should parse permissions from settings")
	assert.Contains(t, perms.Allow, "Bash(*)",
		"permissions allow should contain Bash(*) after approve")
	assert.Contains(t, perms.Deny, "rm -rf",
		"permissions deny should contain rm -rf after approve")

	// Verify MCP is now in .mcp.json.
	m2MCP, err := claudecode.ReadMCPConfig(machine2Claude)
	require.NoError(t, err, "should read MCP config after approve")
	assert.Contains(t, m2MCP, "test-server",
		"MCP should contain test-server after approve")

	// Verify pending-changes.yaml is cleared.
	pendingAfter, err := approval.ReadPending(machine2Sync)
	require.NoError(t, err, "should read pending after approve")
	assert.True(t, pendingAfter.IsEmpty(),
		"pending changes should be empty after approve")
}
