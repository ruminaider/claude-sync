package commands_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/ruminaider/claude-sync/internal/commands"
	"github.com/ruminaider/claude-sync/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupStatusEnv(t *testing.T) (claudeDir, syncDir string) {
	t.Helper()
	claudeDir, syncDir = setupTestEnv(t)
	_, err := commands.Init(claudeDir, syncDir, "")
	require.NoError(t, err)
	return claudeDir, syncDir
}

func TestStatus(t *testing.T) {
	claudeDir, syncDir := setupStatusEnv(t)

	result, err := commands.Status(claudeDir, syncDir)
	require.NoError(t, err)

	assert.Contains(t, result.Synced, "context7@claude-plugins-official")
	assert.Contains(t, result.Synced, "beads@beads-marketplace")
	assert.Empty(t, result.NotInstalled)
	assert.Empty(t, result.Untracked)
}

func TestStatus_NotInstalled(t *testing.T) {
	claudeDir, syncDir := setupStatusEnv(t)

	cfgPath := filepath.Join(syncDir, "config.yaml")
	cfgData, _ := os.ReadFile(cfgPath)
	cfg, _ := config.Parse(cfgData)
	cfg.Upstream = append(cfg.Upstream, "new-plugin@some-marketplace")
	newCfgData, _ := config.Marshal(cfg)
	os.WriteFile(cfgPath, newCfgData, 0644)

	result, err := commands.Status(claudeDir, syncDir)
	require.NoError(t, err)
	assert.Contains(t, result.NotInstalled, "new-plugin@some-marketplace")
}

func TestStatus_Untracked(t *testing.T) {
	claudeDir, syncDir := setupStatusEnv(t)

	pluginsPath := filepath.Join(claudeDir, "plugins", "installed_plugins.json")
	data := `{
		"version": 2,
		"plugins": {
			"context7@claude-plugins-official": [{"scope":"user","installPath":"/p","version":"1.0","installedAt":"2026-01-01T00:00:00Z","lastUpdated":"2026-01-01T00:00:00Z"}],
			"beads@beads-marketplace": [{"scope":"user","installPath":"/p","version":"0.44","installedAt":"2026-01-01T00:00:00Z","lastUpdated":"2026-01-01T00:00:00Z"}],
			"extra-plugin@local": [{"scope":"user","installPath":"/p","version":"1.0","installedAt":"2026-01-01T00:00:00Z","lastUpdated":"2026-01-01T00:00:00Z"}]
		}
	}`
	os.WriteFile(pluginsPath, []byte(data), 0644)

	result, err := commands.Status(claudeDir, syncDir)
	require.NoError(t, err)
	assert.Contains(t, result.Untracked, "extra-plugin@local")
}

func TestStatus_NoSyncDir(t *testing.T) {
	_, err := commands.Status("/tmp/test-claude", "/nonexistent")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not initialized")
}

// setupV2StatusEnv creates a claudeDir with installed plugins and a syncDir
// with a v2 config containing upstream, pinned, and forked plugins.
func setupV2StatusEnv(t *testing.T) (claudeDir, syncDir string) {
	t.Helper()
	claudeDir = t.TempDir()
	syncDir = t.TempDir()

	// Create installed plugins
	pluginDir := filepath.Join(claudeDir, "plugins")
	os.MkdirAll(pluginDir, 0755)

	installedPlugins := `{
		"version": 2,
		"plugins": {
			"context7@claude-plugins-official": [{"scope":"user","installPath":"/p","version":"1.2.0","installedAt":"2026-01-01T00:00:00Z","lastUpdated":"2026-01-01T00:00:00Z"}],
			"beads@beads-marketplace": [{"scope":"user","installPath":"/p","version":"0.44","installedAt":"2026-01-01T00:00:00Z","lastUpdated":"2026-01-01T00:00:00Z"}],
			"my-fork@claude-sync-forks": [{"scope":"user","installPath":"/p","version":"0.1.0","installedAt":"2026-01-01T00:00:00Z","lastUpdated":"2026-01-01T00:00:00Z"}]
		}
	}`
	os.WriteFile(filepath.Join(pluginDir, "installed_plugins.json"), []byte(installedPlugins), 0644)

	// Create v2 config
	cfg := config.ConfigV2{
		Version:  "2.0.0",
		Upstream: []string{"context7@claude-plugins-official"},
		Pinned:   map[string]string{"beads@beads-marketplace": "0.44"},
		Forked:   []string{"my-fork"},
	}
	cfgData, err := config.MarshalV2(cfg)
	require.NoError(t, err)
	os.WriteFile(filepath.Join(syncDir, "config.yaml"), cfgData, 0644)

	return claudeDir, syncDir
}

func TestStatusV2(t *testing.T) {
	claudeDir, syncDir := setupV2StatusEnv(t)

	result, err := commands.Status(claudeDir, syncDir)
	require.NoError(t, err)

	assert.Equal(t, "2.0.0", result.ConfigVersion)

	// Upstream: context7 is installed
	require.Len(t, result.UpstreamSynced, 1)
	assert.Equal(t, "context7@claude-plugins-official", result.UpstreamSynced[0].Key)
	assert.Equal(t, "1.2.0", result.UpstreamSynced[0].InstalledVersion)
	assert.True(t, result.UpstreamSynced[0].Installed)
	assert.Empty(t, result.UpstreamMissing)

	// Pinned: beads is installed
	require.Len(t, result.PinnedSynced, 1)
	assert.Equal(t, "beads@beads-marketplace", result.PinnedSynced[0].Key)
	assert.Equal(t, "0.44", result.PinnedSynced[0].InstalledVersion)
	assert.Equal(t, "0.44", result.PinnedSynced[0].PinnedVersion)
	assert.True(t, result.PinnedSynced[0].Installed)
	assert.Empty(t, result.PinnedMissing)

	// Forked: my-fork is installed (as my-fork@claude-sync-forks)
	require.Len(t, result.ForkedSynced, 1)
	assert.Equal(t, "my-fork", result.ForkedSynced[0].Key)
	assert.Equal(t, "0.1.0", result.ForkedSynced[0].InstalledVersion)
	assert.True(t, result.ForkedSynced[0].Installed)
	assert.Empty(t, result.ForkedMissing)
}

func TestStatusV2_Missing(t *testing.T) {
	claudeDir, syncDir := setupV2StatusEnv(t)

	// Add a missing upstream and missing pinned plugin to config
	cfgPath := filepath.Join(syncDir, "config.yaml")
	cfgData, _ := os.ReadFile(cfgPath)
	cfg, _ := config.Parse(cfgData)
	cfg.Upstream = append(cfg.Upstream, "new-upstream@some-mp")
	cfg.Pinned["missing-pinned@other-mp"] = "1.0.0"
	cfg.Forked = append(cfg.Forked, "missing-fork")
	newData, _ := config.MarshalV2(cfg)
	os.WriteFile(cfgPath, newData, 0644)

	result, err := commands.Status(claudeDir, syncDir)
	require.NoError(t, err)

	// Verify missing entries
	require.Len(t, result.UpstreamMissing, 1)
	assert.Equal(t, "new-upstream@some-mp", result.UpstreamMissing[0].Key)
	assert.False(t, result.UpstreamMissing[0].Installed)

	require.Len(t, result.PinnedMissing, 1)
	assert.Equal(t, "missing-pinned@other-mp", result.PinnedMissing[0].Key)
	assert.Equal(t, "1.0.0", result.PinnedMissing[0].PinnedVersion)
	assert.False(t, result.PinnedMissing[0].Installed)

	require.Len(t, result.ForkedMissing, 1)
	assert.Equal(t, "missing-fork", result.ForkedMissing[0].Key)
	assert.False(t, result.ForkedMissing[0].Installed)
}

func TestStatusJSON(t *testing.T) {
	claudeDir, syncDir := setupV2StatusEnv(t)

	result, err := commands.Status(claudeDir, syncDir)
	require.NoError(t, err)

	data, err := result.JSON()
	require.NoError(t, err)

	// Parse back to verify structure
	var parsed map[string]json.RawMessage
	err = json.Unmarshal(data, &parsed)
	require.NoError(t, err)

	assert.Contains(t, parsed, "config_version")
	assert.Contains(t, parsed, "synced")
	assert.Contains(t, parsed, "not_installed")
	assert.Contains(t, parsed, "untracked")
	assert.Contains(t, parsed, "upstream_synced")
	assert.Contains(t, parsed, "pinned_synced")
	assert.Contains(t, parsed, "forked_synced")

	// Verify config_version value
	var version string
	json.Unmarshal(parsed["config_version"], &version)
	assert.Equal(t, "2.0.0", version)

	// Verify upstream_synced content
	var upstreamSynced []commands.PluginStatus
	json.Unmarshal(parsed["upstream_synced"], &upstreamSynced)
	require.Len(t, upstreamSynced, 1)
	assert.Equal(t, "context7@claude-plugins-official", upstreamSynced[0].Key)
	assert.Equal(t, "1.2.0", upstreamSynced[0].InstalledVersion)
}

func TestStatusHasPendingChanges(t *testing.T) {
	r := &commands.StatusResult{}
	assert.False(t, r.HasPendingChanges())

	r.NotInstalled = []string{"something"}
	assert.True(t, r.HasPendingChanges())

	r.NotInstalled = nil
	r.Untracked = []string{"something"}
	assert.True(t, r.HasPendingChanges())
}
