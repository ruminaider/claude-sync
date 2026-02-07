package commands_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/ruminaider/claude-sync/internal/commands"
	"github.com/ruminaider/claude-sync/internal/config"
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

	err := commands.PushApply(claudeDir, syncDir, []string{"new-plugin@local"}, nil, "Add new plugin")
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
		Version: "2.0.0",
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

	err := commands.PushApply(claudeDir, syncDir, []string{"extra-plugin@local"}, nil, "Add extra plugin")
	require.NoError(t, err)

	// Re-read the config and verify.
	cfgData, err := os.ReadFile(filepath.Join(syncDir, "config.yaml"))
	require.NoError(t, err)
	cfg, err := config.Parse(cfgData)
	require.NoError(t, err)

	// New plugin should be in upstream.
	assert.Contains(t, cfg.Upstream, "extra-plugin@local")
	// Config should still be v2.
	assert.Equal(t, "2.0.0", cfg.Version)
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
	err = commands.PushApply(claudeDir, syncDir, nil, []string{"pinned-plugin@marketplace"}, "Remove pinned plugin")
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
