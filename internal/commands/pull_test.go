package commands_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/ruminaider/claude-sync/internal/claudecode"
	"github.com/ruminaider/claude-sync/internal/commands"
	"github.com/ruminaider/claude-sync/internal/plugins"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPull_DryRun_NothingToInstall(t *testing.T) {
	claudeDir, syncDir := setupStatusEnv(t)

	result, err := commands.PullDryRun(claudeDir, syncDir)
	require.NoError(t, err)
	assert.Empty(t, result.ToInstall)
}

func TestPull_NoSyncDir(t *testing.T) {
	_, err := commands.PullDryRun("/tmp/test-claude", "/nonexistent")
	assert.Error(t, err)
}

func TestPull_GitPull(t *testing.T) {
	remote := t.TempDir()
	exec.Command("git", "init", "--bare", remote).Run()

	claudeDir, syncDir := setupStatusEnv(t)
	exec.Command("git", "-C", syncDir, "remote", "add", "origin", remote).Run()
	exec.Command("git", "-C", syncDir, "push", "-u", "origin", "master").Run()

	result, err := commands.PullDryRun(claudeDir, syncDir)
	require.NoError(t, err)
	assert.Empty(t, result.ToInstall)
}

func TestPull_RespectsUserPreferences(t *testing.T) {
	claudeDir, syncDir := setupStatusEnv(t)

	prefs := "sync_mode: union\nplugins:\n  unsubscribe:\n    - beads@beads-marketplace\n"
	os.WriteFile(filepath.Join(syncDir, "user-preferences.yaml"), []byte(prefs), 0644)

	result, err := commands.PullDryRun(claudeDir, syncDir)
	require.NoError(t, err)
	assert.NotContains(t, result.EffectiveDesired, "beads@beads-marketplace")
}

func TestPull_ExactMode(t *testing.T) {
	claudeDir, syncDir := setupStatusEnv(t)

	// Add extra plugin to installed
	pluginsPath := filepath.Join(claudeDir, "plugins", "installed_plugins.json")
	data := `{
		"version": 2,
		"plugins": {
			"context7@claude-plugins-official": [{"scope":"user","installPath":"/p","version":"1.0","installedAt":"2026-01-01T00:00:00Z","lastUpdated":"2026-01-01T00:00:00Z"}],
			"beads@beads-marketplace": [{"scope":"user","installPath":"/p","version":"0.44","installedAt":"2026-01-01T00:00:00Z","lastUpdated":"2026-01-01T00:00:00Z"}],
			"extra@local": [{"scope":"user","installPath":"/p","version":"1.0","installedAt":"2026-01-01T00:00:00Z","lastUpdated":"2026-01-01T00:00:00Z"}]
		}
	}`
	os.WriteFile(pluginsPath, []byte(data), 0644)

	// Set exact mode
	prefs := "sync_mode: exact\n"
	os.WriteFile(filepath.Join(syncDir, "user-preferences.yaml"), []byte(prefs), 0644)

	result, err := commands.PullDryRun(claudeDir, syncDir)
	require.NoError(t, err)
	assert.Contains(t, result.ToRemove, "extra@local")
	assert.Empty(t, result.Untracked)
}

// setupV2PullEnv creates a test environment with a v2 config that includes a forked plugin.
// It returns (claudeDir, syncDir) where syncDir contains a git repo with a v2 config.yaml
// listing a forked plugin, and a forked plugin directory with a valid manifest.
func setupV2PullEnv(t *testing.T) (claudeDir, syncDir string) {
	t.Helper()

	// Create claudeDir with plugins dir, installed_plugins.json, known_marketplaces.json
	claudeDir = t.TempDir()
	err := claudecode.Bootstrap(claudeDir)
	require.NoError(t, err)

	// Create syncDir
	syncDir = filepath.Join(t.TempDir(), ".claude-sync")
	require.NoError(t, os.MkdirAll(syncDir, 0755))

	// Write a v2 config.yaml with upstream and forked plugins
	configYAML := `version: "2.0.0"
plugins:
  upstream:
    - context7@claude-plugins-official
  forked:
    - my-custom-tool
`
	require.NoError(t, os.WriteFile(filepath.Join(syncDir, "config.yaml"), []byte(configYAML), 0644))

	// Create the forked plugin directory with a valid manifest
	pluginDir := filepath.Join(syncDir, "plugins", "my-custom-tool", ".claude-plugin")
	require.NoError(t, os.MkdirAll(pluginDir, 0755))
	manifest := `{"name": "my-custom-tool", "version": "1.0.0"}`
	require.NoError(t, os.WriteFile(filepath.Join(pluginDir, "plugin.json"), []byte(manifest), 0644))

	// Initialize a git repo in syncDir
	require.NoError(t, exec.Command("git", "init", syncDir).Run())
	require.NoError(t, exec.Command("git", "-C", syncDir, "add", ".").Run())
	require.NoError(t, exec.Command("git", "-C", syncDir, "commit", "-m", "init").Run())

	return claudeDir, syncDir
}

func TestPull_RegistersLocalMarketplace(t *testing.T) {
	claudeDir, syncDir := setupV2PullEnv(t)

	result, err := commands.PullDryRun(claudeDir, syncDir)
	require.NoError(t, err)

	// Verify known_marketplaces.json contains "claude-sync-forks" entry
	mkts, err := claudecode.ReadMarketplaces(claudeDir)
	require.NoError(t, err)
	assert.Contains(t, mkts, plugins.MarketplaceName)

	// Parse the entry to verify structure
	var entry struct {
		Source struct {
			Source string `json:"source"`
			Path   string `json:"path"`
		} `json:"source"`
	}
	err = json.Unmarshal(mkts[plugins.MarketplaceName], &entry)
	require.NoError(t, err)
	assert.Equal(t, "directory", entry.Source.Source)
	assert.Equal(t, filepath.Join(syncDir, "plugins"), entry.Source.Path)

	// Verify forked plugin key appears in effective desired list
	assert.Contains(t, result.EffectiveDesired, "my-custom-tool@claude-sync-forks")
	// Verify upstream plugin is also present
	assert.Contains(t, result.EffectiveDesired, "context7@claude-plugins-official")
	// Forked plugin should be in ToInstall since it's not installed yet
	assert.Contains(t, result.ToInstall, "my-custom-tool@claude-sync-forks")
}
