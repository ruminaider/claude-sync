package commands_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/ruminaider/claude-sync/internal/commands"
	"github.com/ruminaider/claude-sync/internal/config"
	"github.com/ruminaider/claude-sync/internal/profiles"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPushScan(t *testing.T) {
	claudeDir, syncDir := setupStatusEnv(t)

	pluginsPath := filepath.Join(claudeDir, "plugins", "installed_plugins.json")
	data := `{
		"version": 2,
		"plugins": {
			"context7@claude-plugins-official": [{"scope":"user","installPath":"/p","version":"1.0","installedAt":"2026-01-01T00:00:00Z","lastUpdated":"2026-01-01T00:00:00Z"}],
			"beads@beads-marketplace": [{"scope":"user","installPath":"/p","version":"0.44","installedAt":"2026-01-01T00:00:00Z","lastUpdated":"2026-01-01T00:00:00Z"}],
			"new-plugin@local": [{"scope":"user","installPath":"/p","version":"1.0","installedAt":"2026-01-01T00:00:00Z","lastUpdated":"2026-01-01T00:00:00Z"}]
		}
	}`
	os.WriteFile(pluginsPath, []byte(data), 0644)

	scan, err := commands.PushScan(claudeDir, syncDir)
	require.NoError(t, err)
	assert.Contains(t, scan.AddedPlugins, "new-plugin@local")
}

func TestPushApply(t *testing.T) {
	claudeDir, syncDir := setupStatusEnv(t)

	remote := t.TempDir()
	exec.Command("git", "init", "--bare", remote).Run()
	exec.Command("git", "-C", syncDir, "remote", "add", "origin", remote).Run()
	exec.Command("git", "-C", syncDir, "push", "-u", "origin", "master").Run()

	pluginsPath := filepath.Join(claudeDir, "plugins", "installed_plugins.json")
	data := `{
		"version": 2,
		"plugins": {
			"context7@claude-plugins-official": [{"scope":"user","installPath":"/p","version":"1.0","installedAt":"2026-01-01T00:00:00Z","lastUpdated":"2026-01-01T00:00:00Z"}],
			"beads@beads-marketplace": [{"scope":"user","installPath":"/p","version":"0.44","installedAt":"2026-01-01T00:00:00Z","lastUpdated":"2026-01-01T00:00:00Z"}],
			"new-plugin@local": [{"scope":"user","installPath":"/p","version":"1.0","installedAt":"2026-01-01T00:00:00Z","lastUpdated":"2026-01-01T00:00:00Z"}]
		}
	}`
	os.WriteFile(pluginsPath, []byte(data), 0644)

	err := commands.PushApply(commands.PushApplyOptions{
		ClaudeDir:  claudeDir,
		SyncDir:    syncDir,
		AddPlugins: []string{"new-plugin@local"},
		Message:    "Add new plugin",
	})
	require.NoError(t, err)

	cfgData, _ := os.ReadFile(filepath.Join(syncDir, "config.yaml"))
	cfg, _ := config.Parse(cfgData)
	assert.Contains(t, cfg.Upstream, "new-plugin@local")
}

func TestPushScan_NoChanges(t *testing.T) {
	claudeDir, syncDir := setupStatusEnv(t)

	scan, err := commands.PushScan(claudeDir, syncDir)
	require.NoError(t, err)
	assert.Empty(t, scan.AddedPlugins)
	assert.Empty(t, scan.RemovedPlugins)
}

// setupV2PushEnv creates a claudeDir with installed plugins (including one not in config)
// and a syncDir with a v2 config and git initialized.
func setupV2PushEnv(t *testing.T) (claudeDir, syncDir string) {
	t.Helper()
	claudeDir = t.TempDir()
	syncDir = filepath.Join(t.TempDir(), ".claude-sync")
	require.NoError(t, os.MkdirAll(syncDir, 0755))

	// Create installed plugins with an extra plugin not in the config.
	pluginDir := filepath.Join(claudeDir, "plugins")
	require.NoError(t, os.MkdirAll(pluginDir, 0755))

	plugins := `{
		"version": 2,
		"plugins": {
			"context7@claude-plugins-official": [{"scope":"user","installPath":"/p","version":"1.0","installedAt":"2026-01-01T00:00:00Z","lastUpdated":"2026-01-01T00:00:00Z"}],
			"beads@beads-marketplace": [{"scope":"user","installPath":"/p","version":"0.44","installedAt":"2026-01-01T00:00:00Z","lastUpdated":"2026-01-01T00:00:00Z"}],
			"extra-plugin@local": [{"scope":"user","installPath":"/p","version":"1.0","installedAt":"2026-01-01T00:00:00Z","lastUpdated":"2026-01-01T00:00:00Z"}]
		}
	}`
	os.WriteFile(filepath.Join(pluginDir, "installed_plugins.json"), []byte(plugins), 0644)

	// Create v2 config with upstream and pinned plugins.
	cfg := config.ConfigV2{
		Version: "1.0.0",
		Upstream: []string{
			"beads@beads-marketplace",
			"context7@claude-plugins-official",
		},
		Pinned: map[string]string{},
		Forked: []string{},
	}

	cfgData, err := config.MarshalV2(cfg)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(syncDir, "config.yaml"), cfgData, 0644))

	// Initialize git repo with initial commit.
	cmd := exec.Command("git", "init", syncDir)
	require.NoError(t, cmd.Run())
	cmd = exec.Command("git", "-C", syncDir, "config", "user.email", "test@test.com")
	require.NoError(t, cmd.Run())
	cmd = exec.Command("git", "-C", syncDir, "config", "user.name", "Test")
	require.NoError(t, cmd.Run())
	cmd = exec.Command("git", "-C", syncDir, "add", ".")
	require.NoError(t, cmd.Run())
	cmd = exec.Command("git", "-C", syncDir, "commit", "-m", "Initial v2 config")
	require.NoError(t, cmd.Run())

	return claudeDir, syncDir
}

func TestPushScan_V2(t *testing.T) {
	claudeDir, syncDir := setupV2PushEnv(t)

	scan, err := commands.PushScan(claudeDir, syncDir)
	require.NoError(t, err)

	// extra-plugin@local is installed but not in the v2 config — should be detected as added.
	assert.Contains(t, scan.AddedPlugins, "extra-plugin@local")
	assert.Empty(t, scan.RemovedPlugins)
}

func TestPushApply_V2(t *testing.T) {
	_, syncDir := setupV2PushEnv(t)

	// Use a separate claudeDir (not strictly needed, but PushApply requires it).
	claudeDir := filepath.Dir(syncDir)

	err := commands.PushApply(commands.PushApplyOptions{
		ClaudeDir:  claudeDir,
		SyncDir:    syncDir,
		AddPlugins: []string{"extra-plugin@local"},
		Message:    "Add extra plugin",
	})
	require.NoError(t, err)

	// Re-read the config and verify.
	cfgData, err := os.ReadFile(filepath.Join(syncDir, "config.yaml"))
	require.NoError(t, err)
	cfg, err := config.Parse(cfgData)
	require.NoError(t, err)

	// New plugin should be in upstream.
	assert.Contains(t, cfg.Upstream, "extra-plugin@local")
	assert.Equal(t, "1.0.0", cfg.Version)
	// Original plugins should still be present.
	assert.Contains(t, cfg.Upstream, "context7@claude-plugins-official")
	assert.Contains(t, cfg.Upstream, "beads@beads-marketplace")
}

func TestPushApply_V2_RemoveFromPinned(t *testing.T) {
	_, syncDir := setupV2PushEnv(t)
	claudeDir := filepath.Dir(syncDir)

	// First, add a pinned plugin to the config.
	cfgPath := filepath.Join(syncDir, "config.yaml")
	cfgData, err := os.ReadFile(cfgPath)
	require.NoError(t, err)
	cfg, err := config.Parse(cfgData)
	require.NoError(t, err)

	cfg.Pinned["pinned-plugin@marketplace"] = "1.0.0"
	newData, err := config.Marshal(cfg)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(cfgPath, newData, 0644))

	// Commit the change so PushApply can make a clean commit.
	exec.Command("git", "-C", syncDir, "add", ".").Run()
	exec.Command("git", "-C", syncDir, "commit", "-m", "Add pinned plugin").Run()

	// Now remove the pinned plugin via PushApply.
	err = commands.PushApply(commands.PushApplyOptions{
		ClaudeDir:     claudeDir,
		SyncDir:       syncDir,
		RemovePlugins: []string{"pinned-plugin@marketplace"},
		Message:       "Remove pinned plugin",
	})
	require.NoError(t, err)

	// Re-read and verify the pinned plugin was removed.
	cfgData, err = os.ReadFile(cfgPath)
	require.NoError(t, err)
	cfg, err = config.Parse(cfgData)
	require.NoError(t, err)

	_, isPinned := cfg.Pinned["pinned-plugin@marketplace"]
	assert.False(t, isPinned, "pinned plugin should be removed from Pinned")
	assert.NotContains(t, cfg.Upstream, "pinned-plugin@marketplace", "pinned plugin should not be in Upstream")
}

func TestPushApply_ToProfile(t *testing.T) {
	_, syncDir := setupV2PushEnv(t)
	claudeDir := filepath.Dir(syncDir)

	// Create a profile file.
	profilesDir := filepath.Join(syncDir, "profiles")
	require.NoError(t, os.MkdirAll(profilesDir, 0755))
	profileData := []byte("plugins:\n  add:\n    - existing-tool@marketplace\n")
	require.NoError(t, os.WriteFile(filepath.Join(profilesDir, "work.yaml"), profileData, 0644))

	exec.Command("git", "-C", syncDir, "add", ".").Run()
	exec.Command("git", "-C", syncDir, "commit", "-m", "Add work profile").Run()

	err := commands.PushApply(commands.PushApplyOptions{
		ClaudeDir:     claudeDir,
		SyncDir:       syncDir,
		AddPlugins:    []string{"extra-plugin@local"},
		ProfileTarget: "work",
		Message:       "Add extra to work profile",
	})
	require.NoError(t, err)

	// Plugin should NOT be in base upstream.
	cfgData, err := os.ReadFile(filepath.Join(syncDir, "config.yaml"))
	require.NoError(t, err)
	cfg, err := config.Parse(cfgData)
	require.NoError(t, err)
	assert.NotContains(t, cfg.Upstream, "extra-plugin@local")

	// Plugin should be in profile's Plugins.Add.
	profile, err := profiles.ReadProfile(syncDir, "work")
	require.NoError(t, err)
	assert.Contains(t, profile.Plugins.Add, "extra-plugin@local")
	assert.Contains(t, profile.Plugins.Add, "existing-tool@marketplace", "existing plugins preserved")
}

func TestPushApply_ExcludesPlugins(t *testing.T) {
	_, syncDir := setupV2PushEnv(t)
	claudeDir := filepath.Dir(syncDir)

	err := commands.PushApply(commands.PushApplyOptions{
		ClaudeDir:      claudeDir,
		SyncDir:        syncDir,
		AddPlugins:     []string{"extra-plugin@local"},
		ExcludePlugins: []string{"unwanted-a@mkt", "unwanted-b@mkt"},
		Message:        "Add extra, exclude others",
	})
	require.NoError(t, err)

	cfgData, err := os.ReadFile(filepath.Join(syncDir, "config.yaml"))
	require.NoError(t, err)
	cfg, err := config.Parse(cfgData)
	require.NoError(t, err)

	assert.Contains(t, cfg.Upstream, "extra-plugin@local")
	assert.Contains(t, cfg.Excluded, "unwanted-a@mkt")
	assert.Contains(t, cfg.Excluded, "unwanted-b@mkt")
}

func TestPushScan_Permissions(t *testing.T) {
	claudeDir, syncDir := setupV2PushEnv(t)

	// Write settings.json with permissions.
	settings := map[string]any{
		"permissions": map[string]any{
			"allow": []string{"Bash(git *)"},
			"deny":  []string{"Bash(rm *)"},
		},
	}
	settingsData, _ := json.MarshalIndent(settings, "", "  ")
	os.WriteFile(filepath.Join(claudeDir, "settings.json"), settingsData, 0644)

	// Config has no permissions by default, so a diff should be detected.
	scan, err := commands.PushScan(claudeDir, syncDir)
	require.NoError(t, err)
	assert.True(t, scan.ChangedPermissions)
	assert.True(t, scan.HasChanges())
}

func TestPushScan_NoChanges_AllSurfaces(t *testing.T) {
	claudeDir, syncDir := setupV2PushEnv(t)

	// No settings.json, no CLAUDE.md, no MCP, no keybindings.
	// Remove settings.json if it exists (setupV2PushEnv doesn't create one).

	scan, err := commands.PushScan(claudeDir, syncDir)
	require.NoError(t, err)

	// Only plugin difference expected (extra-plugin@local), but no surface changes.
	assert.False(t, scan.ChangedPermissions)
	assert.Nil(t, scan.ChangedClaudeMD)
	assert.False(t, scan.ChangedMCP)
	assert.False(t, scan.ChangedKeybindings)
}

func TestPushScan_MCP(t *testing.T) {
	claudeDir, syncDir := setupV2PushEnv(t)

	// Write .mcp.json with a server.
	mcpData := `{"mcpServers": {"my-server": {"command": "node", "args": ["server.js"]}}}`
	os.WriteFile(filepath.Join(claudeDir, ".mcp.json"), []byte(mcpData), 0644)

	scan, err := commands.PushScan(claudeDir, syncDir)
	require.NoError(t, err)
	assert.True(t, scan.ChangedMCP)
}

func TestPushScan_Keybindings(t *testing.T) {
	claudeDir, syncDir := setupV2PushEnv(t)

	// Write keybindings.json.
	kbData := `{"ctrl+k": "clearScreen"}`
	os.WriteFile(filepath.Join(claudeDir, "keybindings.json"), []byte(kbData), 0644)

	scan, err := commands.PushScan(claudeDir, syncDir)
	require.NoError(t, err)
	assert.True(t, scan.ChangedKeybindings)
}

func TestPushApply_UpdatePermissions(t *testing.T) {
	_, syncDir := setupV2PushEnv(t)
	claudeDir := filepath.Dir(syncDir)

	// Write settings.json with permissions.
	settings := map[string]any{
		"permissions": map[string]any{
			"allow": []string{"Bash(git *)"},
		},
	}
	settingsData, _ := json.MarshalIndent(settings, "", "  ")
	os.WriteFile(filepath.Join(claudeDir, "settings.json"), settingsData, 0644)

	err := commands.PushApply(commands.PushApplyOptions{
		ClaudeDir:         claudeDir,
		SyncDir:           syncDir,
		UpdatePermissions: true,
		Message:           "Update permissions",
	})
	require.NoError(t, err)

	cfgData, err := os.ReadFile(filepath.Join(syncDir, "config.yaml"))
	require.NoError(t, err)
	cfg, err := config.Parse(cfgData)
	require.NoError(t, err)
	assert.Contains(t, cfg.Permissions.Allow, "Bash(git *)")
}

func TestPushApply_UpdateMCP(t *testing.T) {
	_, syncDir := setupV2PushEnv(t)
	claudeDir := filepath.Dir(syncDir)

	// Write .mcp.json.
	mcpData := `{"mcpServers": {"test-server": {"command": "test"}}}`
	os.WriteFile(filepath.Join(claudeDir, ".mcp.json"), []byte(mcpData), 0644)

	err := commands.PushApply(commands.PushApplyOptions{
		ClaudeDir: claudeDir,
		SyncDir:   syncDir,
		UpdateMCP: true,
		Message:   "Update MCP",
	})
	require.NoError(t, err)

	cfgData, err := os.ReadFile(filepath.Join(syncDir, "config.yaml"))
	require.NoError(t, err)
	cfg, err := config.Parse(cfgData)
	require.NoError(t, err)
	assert.Contains(t, cfg.MCP, "test-server")
}
