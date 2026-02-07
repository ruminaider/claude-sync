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
	assert.Contains(t, cfg.Plugins, "new-plugin@local")
}

func TestPushScan_NoChanges(t *testing.T) {
	claudeDir, syncDir := setupStatusEnv(t)

	scan, err := commands.PushScan(claudeDir, syncDir)
	require.NoError(t, err)
	assert.Empty(t, scan.AddedPlugins)
	assert.Empty(t, scan.RemovedPlugins)
}
