package claudecode_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/ruminaider/claude-sync/internal/claudecode"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupClaudeDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	pluginDir := filepath.Join(dir, "plugins")
	os.MkdirAll(pluginDir, 0755)
	return dir
}

func TestReadInstalledPlugins(t *testing.T) {
	dir := setupClaudeDir(t)
	data := `{
		"version": 2,
		"plugins": {
			"context7@claude-plugins-official": [{"scope":"user","installPath":"/p","version":"1.0.0","installedAt":"2026-01-05T00:00:00.000Z","lastUpdated":"2026-01-05T00:00:00.000Z"}],
			"beads@beads-marketplace": [{"scope":"user","installPath":"/p","version":"0.44.0","installedAt":"2026-01-05T00:00:00.000Z","lastUpdated":"2026-01-05T00:00:00.000Z"}]
		}
	}`
	os.WriteFile(filepath.Join(dir, "plugins", "installed_plugins.json"), []byte(data), 0644)

	plugins, err := claudecode.ReadInstalledPlugins(dir)
	require.NoError(t, err)
	assert.Len(t, plugins.Plugins, 2)
	assert.Contains(t, plugins.Plugins, "context7@claude-plugins-official")
}

func TestReadInstalledPlugins_FileNotFound(t *testing.T) {
	dir := t.TempDir()
	_, err := claudecode.ReadInstalledPlugins(dir)
	assert.Error(t, err)
}

func TestPluginKeys(t *testing.T) {
	dir := setupClaudeDir(t)
	data := `{"version":2,"plugins":{"context7@official":[{"scope":"user","installPath":"/p","version":"1.0","installedAt":"2026-01-01T00:00:00Z","lastUpdated":"2026-01-01T00:00:00Z"}],"beads@beads":[{"scope":"user","installPath":"/p","version":"0.44","installedAt":"2026-01-01T00:00:00Z","lastUpdated":"2026-01-01T00:00:00Z"}]}}`
	os.WriteFile(filepath.Join(dir, "plugins", "installed_plugins.json"), []byte(data), 0644)

	plugins, err := claudecode.ReadInstalledPlugins(dir)
	require.NoError(t, err)
	keys := plugins.PluginKeys()
	assert.ElementsMatch(t, []string{"context7@official", "beads@beads"}, keys)
}

func TestReadSettings(t *testing.T) {
	dir := setupClaudeDir(t)
	data := `{"hooks":{"PreCompact":[{"matcher":"","hooks":[{"type":"command","command":"bd prime"}]}]},"enabledPlugins":{"beads@beads-marketplace":true}}`
	os.WriteFile(filepath.Join(dir, "settings.json"), []byte(data), 0644)

	settings, err := claudecode.ReadSettings(dir)
	require.NoError(t, err)
	assert.Contains(t, settings, "hooks")
	assert.Contains(t, settings, "enabledPlugins")
}

func TestReadMarketplaces(t *testing.T) {
	dir := setupClaudeDir(t)
	data := `{"claude-plugins-official":{"source":{"source":"github","repo":"anthropics/claude-plugins-official"},"installLocation":"/path","lastUpdated":"2026-01-01T00:00:00.000Z"}}`
	os.WriteFile(filepath.Join(dir, "plugins", "known_marketplaces.json"), []byte(data), 0644)

	mkts, err := claudecode.ReadMarketplaces(dir)
	require.NoError(t, err)
	assert.Contains(t, mkts, "claude-plugins-official")
}

func TestWriteMarketplaces(t *testing.T) {
	dir := setupClaudeDir(t)
	mkts := map[string]json.RawMessage{
		"test-marketplace": json.RawMessage(`{"source":{"source":"directory","path":"/test"},"installLocation":"/test","lastUpdated":"2026-01-01T00:00:00Z"}`),
	}
	err := claudecode.WriteMarketplaces(dir, mkts)
	require.NoError(t, err)

	readBack, err := claudecode.ReadMarketplaces(dir)
	require.NoError(t, err)
	assert.Contains(t, readBack, "test-marketplace")
}

func TestDirExists(t *testing.T) {
	t.Run("exists", func(t *testing.T) {
		dir := setupClaudeDir(t)
		assert.True(t, claudecode.DirExists(dir))
	})
	t.Run("not exists", func(t *testing.T) {
		assert.False(t, claudecode.DirExists("/nonexistent"))
	})
}

func TestBootstrap(t *testing.T) {
	dir := filepath.Join(t.TempDir(), ".claude")
	err := claudecode.Bootstrap(dir)
	require.NoError(t, err)

	// Verify files were created
	_, err = os.Stat(filepath.Join(dir, "plugins", "installed_plugins.json"))
	assert.NoError(t, err)
	_, err = os.Stat(filepath.Join(dir, "plugins", "known_marketplaces.json"))
	assert.NoError(t, err)
}
