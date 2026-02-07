package commands_test

import (
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
	err := commands.Init(claudeDir, syncDir)
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
	cfg.Plugins = append(cfg.Plugins, "new-plugin@some-marketplace")
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
