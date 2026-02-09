package plugins_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/ruminaider/claude-sync/internal/claudecode"
	"github.com/ruminaider/claude-sync/internal/plugins"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupClaudeDir creates a temp directory with the minimal Claude Code structure
// (plugins/ directory and an empty known_marketplaces.json).
func setupClaudeDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	err := claudecode.Bootstrap(dir)
	require.NoError(t, err)
	return dir
}

// setupSyncDir creates a temp directory to act as the sync repo root.
func setupSyncDir(t *testing.T) string {
	t.Helper()
	return t.TempDir()
}

// createForkedPlugin creates a minimal forked plugin directory structure
// at <syncDir>/plugins/<name>/.claude-plugin/plugin.json.
func createForkedPlugin(t *testing.T, syncDir, name string) {
	t.Helper()
	pluginDir := filepath.Join(syncDir, "plugins", name, ".claude-plugin")
	require.NoError(t, os.MkdirAll(pluginDir, 0755))
	manifest := `{"name": "` + name + `", "version": "1.0.0"}`
	require.NoError(t, os.WriteFile(filepath.Join(pluginDir, "plugin.json"), []byte(manifest), 0644))
}

func TestRegisterLocalMarketplace(t *testing.T) {
	claudeDir := setupClaudeDir(t)
	syncDir := setupSyncDir(t)

	err := plugins.RegisterLocalMarketplace(claudeDir, syncDir)
	require.NoError(t, err)

	// Read back and verify the entry was created.
	mkts, err := claudecode.ReadMarketplaces(claudeDir)
	require.NoError(t, err)
	assert.Contains(t, mkts, plugins.MarketplaceName)

	// Parse the entry to verify its structure.
	var entry struct {
		Source struct {
			Source string `json:"source"`
			Path   string `json:"path"`
		} `json:"source"`
		InstallLocation string `json:"installLocation"`
		LastUpdated     string `json:"lastUpdated"`
	}
	err = json.Unmarshal(mkts[plugins.MarketplaceName], &entry)
	require.NoError(t, err)

	assert.Equal(t, "directory", entry.Source.Source)
	assert.Equal(t, filepath.Join(syncDir, "plugins"), entry.Source.Path)
	assert.Equal(t, filepath.Join(syncDir, "plugins"), entry.InstallLocation)
	assert.NotEmpty(t, entry.LastUpdated)
}

func TestRegisterLocalMarketplace_PreservesExisting(t *testing.T) {
	claudeDir := setupClaudeDir(t)
	syncDir := setupSyncDir(t)

	// Write an existing marketplace entry first.
	existingEntry := json.RawMessage(`{"source":{"source":"github","repo":"anthropics/claude-plugins-official"},"installLocation":"/path/to/official","lastUpdated":"2026-01-01T00:00:00.000Z"}`)
	mkts := map[string]json.RawMessage{
		"claude-plugins-official": existingEntry,
	}
	err := claudecode.WriteMarketplaces(claudeDir, mkts)
	require.NoError(t, err)

	// Register the local marketplace.
	err = plugins.RegisterLocalMarketplace(claudeDir, syncDir)
	require.NoError(t, err)

	// Read back and verify both entries exist.
	mkts, err = claudecode.ReadMarketplaces(claudeDir)
	require.NoError(t, err)
	assert.Contains(t, mkts, "claude-plugins-official", "existing marketplace should be preserved")
	assert.Contains(t, mkts, plugins.MarketplaceName, "claude-sync-forks should be added")
	assert.Len(t, mkts, 2)
}

func TestListForkedPlugins(t *testing.T) {
	syncDir := setupSyncDir(t)

	// Create two valid forked plugins.
	createForkedPlugin(t, syncDir, "my-custom-plugin")
	createForkedPlugin(t, syncDir, "another-plugin")

	// Create a directory without a manifest (should be skipped).
	invalidDir := filepath.Join(syncDir, "plugins", "not-a-plugin")
	require.NoError(t, os.MkdirAll(invalidDir, 0755))

	// Create a regular file in the plugins dir (should be skipped).
	require.NoError(t, os.WriteFile(filepath.Join(syncDir, "plugins", "README.md"), []byte("# Plugins"), 0644))

	found, err := plugins.ListForkedPlugins(syncDir)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"my-custom-plugin", "another-plugin"}, found)
}

func TestListForkedPlugins_EmptyDir(t *testing.T) {
	syncDir := setupSyncDir(t)
	// No plugins directory exists at all.

	found, err := plugins.ListForkedPlugins(syncDir)
	require.NoError(t, err)
	assert.Empty(t, found)
	assert.NotNil(t, found, "should return empty slice, not nil")
}

func TestListForkedPlugins_EmptyPluginsDir(t *testing.T) {
	syncDir := setupSyncDir(t)
	// Create an empty plugins directory.
	require.NoError(t, os.MkdirAll(filepath.Join(syncDir, "plugins"), 0755))

	found, err := plugins.ListForkedPlugins(syncDir)
	require.NoError(t, err)
	assert.Empty(t, found)
	assert.NotNil(t, found, "should return empty slice, not nil")
}

func TestForkedPluginKey(t *testing.T) {
	assert.Equal(t, "my-plugin@claude-sync-forks", plugins.ForkedPluginKey("my-plugin"))
	assert.Equal(t, "context7@claude-sync-forks", plugins.ForkedPluginKey("context7"))
}

func TestMarketplaceName(t *testing.T) {
	assert.Equal(t, "claude-sync-forks", plugins.MarketplaceName)
}

func TestRegisterLocalMarketplace_GeneratesManifest(t *testing.T) {
	claudeDir := setupClaudeDir(t)
	syncDir := setupSyncDir(t)

	// Create two forked plugins.
	createForkedPlugin(t, syncDir, "plugin-a")
	createForkedPlugin(t, syncDir, "plugin-b")

	err := plugins.RegisterLocalMarketplace(claudeDir, syncDir)
	require.NoError(t, err)

	// Verify marketplace.json was generated.
	manifestPath := filepath.Join(syncDir, "plugins", ".claude-plugin", "marketplace.json")
	data, err := os.ReadFile(manifestPath)
	require.NoError(t, err)

	var manifest struct {
		Schema string `json:"$schema"`
		Name   string `json:"name"`
		Owner  struct {
			Name string `json:"name"`
		} `json:"owner"`
		Plugins []struct {
			Name    string `json:"name"`
			Source  string `json:"source"`
			Version string `json:"version"`
		} `json:"plugins"`
	}
	err = json.Unmarshal(data, &manifest)
	require.NoError(t, err)

	assert.Equal(t, "claude-sync-forks", manifest.Name)
	assert.Equal(t, "claude-sync", manifest.Owner.Name)
	assert.Len(t, manifest.Plugins, 2)

	// Verify plugin entries have correct source paths.
	names := make(map[string]string)
	for _, p := range manifest.Plugins {
		names[p.Name] = p.Source
	}
	assert.Equal(t, "./plugin-a", names["plugin-a"])
	assert.Equal(t, "./plugin-b", names["plugin-b"])
}
