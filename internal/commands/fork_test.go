package commands_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/ruminaider/claude-sync/internal/commands"
	"github.com/ruminaider/claude-sync/internal/config"
	"github.com/ruminaider/claude-sync/internal/git"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupForkTestEnv creates a claudeDir with a cached plugin and a syncDir with
// a v2 config listing the plugin as upstream, plus an initialized git repo.
func setupForkTestEnv(t *testing.T) (claudeDir, syncDir string) {
	t.Helper()

	claudeDir = t.TempDir()
	syncDir = filepath.Join(t.TempDir(), ".claude-sync")

	// Create a cached plugin directory with files.
	marketplace := "test-marketplace"
	pluginName := "test-plugin"
	pluginCacheDir := filepath.Join(claudeDir, "plugins", "marketplaces", marketplace, pluginName)
	require.NoError(t, os.MkdirAll(pluginCacheDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(pluginCacheDir, "manifest.json"), []byte(`{"name":"test-plugin"}`), 0644))
	require.NoError(t, os.MkdirAll(filepath.Join(pluginCacheDir, "src"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(pluginCacheDir, "src", "index.js"), []byte(`console.log("hello")`), 0644))

	// Create installed_plugins.json pointing to the cache dir.
	pluginsDir := filepath.Join(claudeDir, "plugins")
	installedPlugins := map[string]any{
		"version": 2,
		"plugins": map[string]any{
			pluginName + "@" + marketplace: []map[string]any{
				{
					"scope":       "user",
					"installPath": pluginCacheDir,
					"version":     "1.0.0",
					"installedAt": "2026-01-01T00:00:00Z",
					"lastUpdated": "2026-01-01T00:00:00Z",
				},
			},
		},
	}
	data, err := json.MarshalIndent(installedPlugins, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(pluginsDir, "installed_plugins.json"), data, 0644))

	// Create known_marketplaces.json.
	require.NoError(t, os.WriteFile(filepath.Join(pluginsDir, "known_marketplaces.json"), []byte("{}"), 0644))

	// Create syncDir with v2 config listing the plugin as upstream.
	require.NoError(t, os.MkdirAll(syncDir, 0755))
	cfg := config.Config{
		Version:  "1.0.0",
		Upstream: []string{pluginName + "@" + marketplace},
		Pinned:   map[string]string{},
	}
	cfgData, err := config.MarshalV2(cfg)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(syncDir, "config.yaml"), cfgData, 0644))

	// Initialize git repo in syncDir.
	require.NoError(t, git.Init(syncDir))
	_, err = git.Run(syncDir, "config", "user.email", "test@test.com")
	require.NoError(t, err)
	_, err = git.Run(syncDir, "config", "user.name", "Test")
	require.NoError(t, err)
	require.NoError(t, git.Add(syncDir, "."))
	require.NoError(t, git.Commit(syncDir, "Initial config"))

	return claudeDir, syncDir
}

func TestFork(t *testing.T) {
	claudeDir, syncDir := setupForkTestEnv(t)

	err := commands.Fork(claudeDir, syncDir, "test-plugin@test-marketplace")
	require.NoError(t, err)

	// Verify plugin files were copied.
	manifest, err := os.ReadFile(filepath.Join(syncDir, "plugins", "test-plugin", "manifest.json"))
	require.NoError(t, err)
	assert.Contains(t, string(manifest), "test-plugin")

	indexJS, err := os.ReadFile(filepath.Join(syncDir, "plugins", "test-plugin", "src", "index.js"))
	require.NoError(t, err)
	assert.Contains(t, string(indexJS), "hello")

	// Verify config was updated.
	cfgData, err := os.ReadFile(filepath.Join(syncDir, "config.yaml"))
	require.NoError(t, err)

	cfg, err := config.Parse(cfgData)
	require.NoError(t, err)

	assert.NotContains(t, cfg.Upstream, "test-plugin@test-marketplace")
	assert.Contains(t, cfg.Forked, "test-plugin")

	// Verify a git commit was made.
	out, err := git.Run(syncDir, "log", "--oneline", "-1")
	require.NoError(t, err)
	assert.Contains(t, out, "Fork test-plugin for customization")
}

func TestFork_InvalidKey(t *testing.T) {
	err := commands.Fork("/tmp", "/tmp", "no-at-sign")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid plugin key")
}

func TestFork_PluginNotInstalled(t *testing.T) {
	claudeDir, syncDir := setupForkTestEnv(t)

	err := commands.Fork(claudeDir, syncDir, "nonexistent@test-marketplace")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestUnfork(t *testing.T) {
	claudeDir, syncDir := setupForkTestEnv(t)

	// First fork the plugin.
	err := commands.Fork(claudeDir, syncDir, "test-plugin@test-marketplace")
	require.NoError(t, err)

	// Verify it was forked.
	_, err = os.Stat(filepath.Join(syncDir, "plugins", "test-plugin"))
	require.NoError(t, err)

	// Now unfork it.
	err = commands.Unfork(syncDir, "test-plugin", "test-marketplace")
	require.NoError(t, err)

	// Verify plugin directory was removed.
	_, err = os.Stat(filepath.Join(syncDir, "plugins", "test-plugin"))
	assert.True(t, os.IsNotExist(err))

	// Verify config was updated.
	cfgData, err := os.ReadFile(filepath.Join(syncDir, "config.yaml"))
	require.NoError(t, err)

	cfg, err := config.Parse(cfgData)
	require.NoError(t, err)

	assert.NotContains(t, cfg.Forked, "test-plugin")
	assert.Contains(t, cfg.Upstream, "test-plugin@test-marketplace")

	// Verify a git commit was made.
	out, err := git.Run(syncDir, "log", "--oneline", "-1")
	require.NoError(t, err)
	assert.Contains(t, out, "Unfork test-plugin")
}
