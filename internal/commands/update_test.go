package commands_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ruminaider/claude-sync/internal/commands"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupUpdateTestEnv creates a claudeDir with installed plugins and a syncDir
// with a v2 config containing upstream, pinned, and forked categories.
func setupUpdateTestEnv(t *testing.T) (claudeDir, syncDir string) {
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
			"beads@beads-marketplace": [{"scope":"user","installPath":"/p","version":"0.44.0","installedAt":"2026-01-01T00:00:00Z","lastUpdated":"2026-01-01T00:00:00Z"}],
			"pinned-tool@some-marketplace": [{"scope":"user","installPath":"/p","version":"2.0.0","installedAt":"2026-01-01T00:00:00Z","lastUpdated":"2026-01-01T00:00:00Z"}]
		}
	}`
	os.WriteFile(filepath.Join(pluginDir, "installed_plugins.json"), []byte(installedPlugins), 0644)
	os.WriteFile(filepath.Join(pluginDir, "known_marketplaces.json"), []byte("{}"), 0644)

	// Create v2 config with upstream, pinned, and forked
	configYAML := `version: "1.0.0"
plugins:
  upstream:
    - context7@claude-plugins-official
    - beads@beads-marketplace
  pinned:
    - pinned-tool@some-marketplace: "1.5.0"
  forked:
    - my-custom-tool
`
	os.WriteFile(filepath.Join(syncDir, "config.yaml"), []byte(configYAML), 0644)

	return claudeDir, syncDir
}

func TestUpdateCheck(t *testing.T) {
	claudeDir, syncDir := setupUpdateTestEnv(t)

	result, err := commands.UpdateCheck(claudeDir, syncDir)
	require.NoError(t, err)

	// Verify upstream plugins are categorized correctly
	assert.Len(t, result.UpstreamPlugins, 2)
	upstreamKeys := make(map[string]string)
	for _, u := range result.UpstreamPlugins {
		upstreamKeys[u.Key] = u.InstalledVersion
	}
	assert.Equal(t, "1.2.0", upstreamKeys["context7@claude-plugins-official"])
	assert.Equal(t, "0.44.0", upstreamKeys["beads@beads-marketplace"])

	// Verify pinned plugins are categorized correctly
	assert.Len(t, result.PinnedPlugins, 1)
	assert.Equal(t, "pinned-tool@some-marketplace", result.PinnedPlugins[0].Key)
	assert.Equal(t, "1.5.0", result.PinnedPlugins[0].PinnedVersion)
	assert.Equal(t, "2.0.0", result.PinnedPlugins[0].InstalledVersion)

	// Verify forked plugins are categorized correctly
	assert.Len(t, result.ForkedPlugins, 1)
	assert.Equal(t, "my-custom-tool", result.ForkedPlugins[0].Name)
}

func TestUpdateCheck_NoSyncDir(t *testing.T) {
	_, err := commands.UpdateCheck("/tmp/test-claude", "/nonexistent")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not initialized")
}

func TestUpdateCheck_UpstreamOnly(t *testing.T) {
	claudeDir := t.TempDir()
	syncDir := t.TempDir()

	pluginDir := filepath.Join(claudeDir, "plugins")
	os.MkdirAll(pluginDir, 0755)

	installedPlugins := `{
		"version": 2,
		"plugins": {
			"context7@claude-plugins-official": [{"scope":"user","installPath":"/p","version":"1.0.0","installedAt":"2026-01-01T00:00:00Z","lastUpdated":"2026-01-01T00:00:00Z"}]
		}
	}`
	os.WriteFile(filepath.Join(pluginDir, "installed_plugins.json"), []byte(installedPlugins), 0644)

	configYAML := `version: "1.0.0"
plugins:
  upstream:
    - context7@claude-plugins-official
`
	os.WriteFile(filepath.Join(syncDir, "config.yaml"), []byte(configYAML), 0644)

	result, err := commands.UpdateCheck(claudeDir, syncDir)
	require.NoError(t, err)

	assert.Len(t, result.UpstreamPlugins, 1)
	assert.Equal(t, "context7@claude-plugins-official", result.UpstreamPlugins[0].Key)
	assert.Equal(t, "1.0.0", result.UpstreamPlugins[0].InstalledVersion)
	assert.Empty(t, result.PinnedPlugins)
	assert.Empty(t, result.ForkedPlugins)
}

func TestUpdateCheck_NotInstalledPlugin(t *testing.T) {
	claudeDir := t.TempDir()
	syncDir := t.TempDir()

	pluginDir := filepath.Join(claudeDir, "plugins")
	os.MkdirAll(pluginDir, 0755)

	// No plugins installed
	installedPlugins := `{"version": 2, "plugins": {}}`
	os.WriteFile(filepath.Join(pluginDir, "installed_plugins.json"), []byte(installedPlugins), 0644)

	configYAML := `version: "1.0.0"
plugins:
  upstream:
    - context7@claude-plugins-official
`
	os.WriteFile(filepath.Join(syncDir, "config.yaml"), []byte(configYAML), 0644)

	result, err := commands.UpdateCheck(claudeDir, syncDir)
	require.NoError(t, err)

	assert.Len(t, result.UpstreamPlugins, 1)
	assert.Equal(t, "", result.UpstreamPlugins[0].InstalledVersion)
}

func TestUpdateResultHasUpdates(t *testing.T) {
	// Empty result has no updates
	empty := &commands.UpdateResult{}
	assert.False(t, empty.HasUpdates())

	// With upstream plugins
	withUpstream := &commands.UpdateResult{
		UpstreamPlugins: []commands.UpstreamStatus{
			{Key: "test@marketplace", InstalledVersion: "1.0"},
		},
	}
	assert.True(t, withUpstream.HasUpdates())

	// With pinned plugins only
	withPinned := &commands.UpdateResult{
		PinnedPlugins: []commands.PinnedStatus{
			{Key: "test@marketplace", PinnedVersion: "1.0", InstalledVersion: "1.0"},
		},
	}
	assert.True(t, withPinned.HasUpdates())

	// With forked plugins only
	withForked := &commands.UpdateResult{
		ForkedPlugins: []commands.ForkedStatus{
			{Name: "my-fork"},
		},
	}
	assert.True(t, withForked.HasUpdates())

	// With all categories empty explicitly
	allEmpty := &commands.UpdateResult{
		UpstreamPlugins: []commands.UpstreamStatus{},
		PinnedPlugins:   []commands.PinnedStatus{},
		ForkedPlugins:   []commands.ForkedStatus{},
	}
	assert.False(t, allEmpty.HasUpdates())
}
