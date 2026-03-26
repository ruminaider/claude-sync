package commands

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/ruminaider/claude-sync/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupBuildConfigEnv creates the minimum directory structure that
// buildAndWriteConfig needs: a claudeDir with installed_plugins.json,
// known_marketplaces.json, and settings.json, plus a fresh syncDir.
func setupBuildConfigEnv(t *testing.T) (claudeDir, syncDir string) {
	t.Helper()
	claudeDir = t.TempDir()
	syncDir = filepath.Join(t.TempDir(), "sync")

	pluginDir := filepath.Join(claudeDir, "plugins")
	require.NoError(t, os.MkdirAll(pluginDir, 0755))

	// Minimal installed plugins (empty list).
	require.NoError(t, os.WriteFile(
		filepath.Join(pluginDir, "installed_plugins.json"),
		[]byte(`{"version":2,"plugins":{}}`), 0644))
	require.NoError(t, os.WriteFile(
		filepath.Join(pluginDir, "known_marketplaces.json"),
		[]byte(`{}`), 0644))
	require.NoError(t, os.WriteFile(
		filepath.Join(claudeDir, "settings.json"),
		[]byte(`{}`), 0644))

	return claudeDir, syncDir
}

func TestBuildAndWriteConfig_ExtraUpstream(t *testing.T) {
	claudeDir, syncDir := setupBuildConfigEnv(t)

	opts := InitOptions{
		ClaudeDir:     claudeDir,
		SyncDir:       syncDir,
		ExtraUpstream: []string{"missing-plugin@some-marketplace"},
	}

	result, _, err := buildAndWriteConfig(opts)
	require.NoError(t, err)

	// The extra upstream key should appear in the result.
	assert.Contains(t, result.Upstream, "missing-plugin@some-marketplace")

	// Read back config and verify.
	cfgData, err := os.ReadFile(filepath.Join(syncDir, "config.yaml"))
	require.NoError(t, err)
	cfg, err := config.Parse(cfgData)
	require.NoError(t, err)

	assert.Contains(t, cfg.Upstream, "missing-plugin@some-marketplace")
}

func TestBuildAndWriteConfig_ExtraUpstream_NoDuplicate(t *testing.T) {
	claudeDir, syncDir := setupBuildConfigEnv(t)

	// Install a plugin locally AND also pass it as ExtraUpstream.
	pluginDir := filepath.Join(claudeDir, "plugins")
	require.NoError(t, os.WriteFile(
		filepath.Join(pluginDir, "installed_plugins.json"),
		[]byte(`{"version":2,"plugins":{"ctx@official":[{"scope":"user","installPath":"/p","version":"1.0","installedAt":"2026-01-01T00:00:00Z","lastUpdated":"2026-01-01T00:00:00Z"}]}}`),
		0644))

	opts := InitOptions{
		ClaudeDir:     claudeDir,
		SyncDir:       syncDir,
		ExtraUpstream: []string{"ctx@official"},
	}

	result, _, err := buildAndWriteConfig(opts)
	require.NoError(t, err)

	// Should only appear once.
	count := 0
	for _, k := range result.Upstream {
		if k == "ctx@official" {
			count++
		}
	}
	assert.Equal(t, 1, count, "extra upstream should not create a duplicate")
}

func TestBuildAndWriteConfig_ExtraForked(t *testing.T) {
	claudeDir, syncDir := setupBuildConfigEnv(t)

	opts := InitOptions{
		ClaudeDir:   claudeDir,
		SyncDir:     syncDir,
		ExtraForked: []string{"my-plugin"},
	}

	result, _, err := buildAndWriteConfig(opts)
	require.NoError(t, err)

	assert.Contains(t, result.AutoForked, "my-plugin@claude-sync-forks")

	cfgData, err := os.ReadFile(filepath.Join(syncDir, "config.yaml"))
	require.NoError(t, err)
	cfg, err := config.Parse(cfgData)
	require.NoError(t, err)

	assert.Contains(t, cfg.Forked, "my-plugin")
}

func TestBuildAndWriteConfig_ExtraForked_NoDuplicate(t *testing.T) {
	claudeDir, syncDir := setupBuildConfigEnv(t)

	// "claude-sync" is always added as a forked plugin by the bundled
	// plugin logic. Passing it as ExtraForked should not duplicate it.
	opts := InitOptions{
		ClaudeDir:   claudeDir,
		SyncDir:     syncDir,
		ExtraForked: []string{"claude-sync"},
	}

	result, forkedNames, err := buildAndWriteConfig(opts)
	require.NoError(t, err)

	count := 0
	for _, n := range forkedNames {
		if n == "claude-sync" {
			count++
		}
	}
	assert.Equal(t, 1, count, "claude-sync should not be duplicated in forkedNames")

	// The bundled plugin should be in forkedNames (for internal use) but NOT in config.yaml.
	cfgData, err := os.ReadFile(filepath.Join(syncDir, "config.yaml"))
	require.NoError(t, err)
	cfg, err := config.Parse(cfgData)
	require.NoError(t, err)
	assert.NotContains(t, cfg.Forked, "claude-sync", "bundled plugin should not appear in config")
	_ = result
}

func TestBuildAndWriteConfig_ExtraSettings(t *testing.T) {
	claudeDir, syncDir := setupBuildConfigEnv(t)

	// Write settings.json with one setting.
	require.NoError(t, os.WriteFile(
		filepath.Join(claudeDir, "settings.json"),
		[]byte(`{"theme":"dark"}`), 0644))

	opts := InitOptions{
		ClaudeDir:       claudeDir,
		SyncDir:         syncDir,
		IncludeSettings: true,
		SettingsFilter:  []string{"theme", "model"},
		ExtraSettings:   map[string]any{"model": "opus"},
	}

	result, _, err := buildAndWriteConfig(opts)
	require.NoError(t, err)

	// Both the locally detected "theme" and the extra "model" should appear.
	assert.True(t, slices.Contains(result.IncludedSettings, "theme"), "locally detected setting should be included")
	assert.True(t, slices.Contains(result.IncludedSettings, "model"), "extra setting should be included")

	cfgData, err := os.ReadFile(filepath.Join(syncDir, "config.yaml"))
	require.NoError(t, err)
	cfg, err := config.Parse(cfgData)
	require.NoError(t, err)

	assert.Equal(t, "dark", cfg.Settings["theme"])
	assert.Equal(t, "opus", cfg.Settings["model"])
}

func TestBuildAndWriteConfig_ExtraSettings_NilCfgSettings(t *testing.T) {
	claudeDir, syncDir := setupBuildConfigEnv(t)

	// IncludeSettings is false, so cfgSettings will be nil before the
	// ExtraSettings merge. ExtraSettings should still get written.
	opts := InitOptions{
		ClaudeDir:     claudeDir,
		SyncDir:       syncDir,
		ExtraSettings: map[string]any{"model": "opus", "verbose": true},
	}

	result, _, err := buildAndWriteConfig(opts)
	require.NoError(t, err)

	assert.Contains(t, result.IncludedSettings, "model")
	assert.Contains(t, result.IncludedSettings, "verbose")

	cfgData, err := os.ReadFile(filepath.Join(syncDir, "config.yaml"))
	require.NoError(t, err)
	cfg, err := config.Parse(cfgData)
	require.NoError(t, err)

	assert.Equal(t, "opus", cfg.Settings["model"])
	assert.Equal(t, true, cfg.Settings["verbose"])
}

func TestBuildAndWriteConfig_ExtraSettings_NoOverrideLocal(t *testing.T) {
	claudeDir, syncDir := setupBuildConfigEnv(t)

	// Local settings.json has "theme": "dark".
	require.NoError(t, os.WriteFile(
		filepath.Join(claudeDir, "settings.json"),
		[]byte(`{"theme":"dark"}`), 0644))

	// ExtraSettings tries to set "theme" to "light", but local should win.
	opts := InitOptions{
		ClaudeDir:       claudeDir,
		SyncDir:         syncDir,
		IncludeSettings: true,
		ExtraSettings:   map[string]any{"theme": "light"},
	}

	result, _, err := buildAndWriteConfig(opts)
	require.NoError(t, err)

	_ = result

	cfgData, err := os.ReadFile(filepath.Join(syncDir, "config.yaml"))
	require.NoError(t, err)
	cfg, err := config.Parse(cfgData)
	require.NoError(t, err)

	// Local value should take precedence.
	assert.Equal(t, "dark", cfg.Settings["theme"])
}
