//go:build integration

package tests

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"testing"

	"github.com/ruminaider/claude-sync/internal/claudecode"
	"github.com/ruminaider/claude-sync/internal/commands"
	"github.com/ruminaider/claude-sync/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupMockClaude creates a minimal Claude Code directory with the given plugins.
// It writes plugins/installed_plugins.json (v2 format), plugins/known_marketplaces.json,
// and settings.json.
func setupMockClaude(t *testing.T, dir string, plugins []string) {
	t.Helper()

	pluginDir := filepath.Join(dir, "plugins")
	require.NoError(t, os.MkdirAll(pluginDir, 0755))

	pluginsMap := make(map[string][]claudecode.PluginInstallation)
	for _, p := range plugins {
		pluginsMap[p] = []claudecode.PluginInstallation{
			{
				Scope:       "user",
				InstallPath: "/mock/" + p,
				Version:     "1.0.0",
				InstalledAt: "2026-01-01T00:00:00Z",
				LastUpdated: "2026-01-01T00:00:00Z",
			},
		}
	}

	installed := claudecode.InstalledPlugins{
		Version: 2,
		Plugins: pluginsMap,
	}

	data, err := json.MarshalIndent(installed, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(pluginDir, "installed_plugins.json"), data, 0644))

	require.NoError(t, os.WriteFile(filepath.Join(pluginDir, "known_marketplaces.json"), []byte("{}"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "settings.json"), []byte("{}"), 0644))
}

// addPluginToMockClaude reads the existing installed_plugins.json, adds a new plugin entry,
// and writes it back.
func addPluginToMockClaude(t *testing.T, dir string, plugin string) {
	t.Helper()

	path := filepath.Join(dir, "plugins", "installed_plugins.json")
	data, err := os.ReadFile(path)
	require.NoError(t, err)

	var installed claudecode.InstalledPlugins
	require.NoError(t, json.Unmarshal(data, &installed))

	if installed.Plugins == nil {
		installed.Plugins = make(map[string][]claudecode.PluginInstallation)
	}

	installed.Plugins[plugin] = []claudecode.PluginInstallation{
		{
			Scope:       "user",
			InstallPath: "/mock/" + plugin,
			Version:     "1.0.0",
			InstalledAt: "2026-01-28T00:00:00Z",
			LastUpdated: "2026-01-28T00:00:00Z",
		},
	}

	newData, err := json.MarshalIndent(installed, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, newData, 0644))
}

// gitRun executes a git command in the given directory and fails the test on error.
func gitRun(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "git %v failed: %s", args, string(out))
	return string(out)
}

func TestFullWorkflow(t *testing.T) {
	// Create all directories using t.TempDir() for automatic cleanup.
	machine1Claude := t.TempDir()
	machine1Sync := filepath.Join(t.TempDir(), ".claude-sync")
	machine2Claude := t.TempDir()
	machine2Sync := filepath.Join(t.TempDir(), ".claude-sync")

	// Create a bare "remote" git repo to simulate a shared origin.
	remoteDir := filepath.Join(t.TempDir(), "remote.git")
	gitRun(t, ".", "init", "--bare", remoteDir)

	// ---------------------------------------------------------------
	// Step 1: Machine 1 — set up mock Claude Code dir with two plugins.
	// ---------------------------------------------------------------
	setupMockClaude(t, machine1Claude, []string{
		"context7@claude-plugins-official",
		"beads@beads-marketplace",
	})

	// ---------------------------------------------------------------
	// Step 2: Machine 1 — Init.
	// ---------------------------------------------------------------
	err := commands.Init(machine1Claude, machine1Sync)
	require.NoError(t, err, "Init should succeed on machine 1")

	// ---------------------------------------------------------------
	// Step 3: Machine 1 — Add remote and push.
	// ---------------------------------------------------------------
	gitRun(t, machine1Sync, "remote", "add", "origin", remoteDir)
	gitRun(t, machine1Sync, "push", "-u", "origin", "HEAD")

	// ---------------------------------------------------------------
	// Step 4: Machine 1 — Status should show 2 synced, 0 not installed.
	// ---------------------------------------------------------------
	status1, err := commands.Status(machine1Claude, machine1Sync)
	require.NoError(t, err, "Status should succeed on machine 1")

	assert.Len(t, status1.Synced, 2, "Machine 1 should have 2 synced plugins")
	assert.Empty(t, status1.NotInstalled, "Machine 1 should have 0 not-installed plugins")

	// ---------------------------------------------------------------
	// Step 5: Machine 2 — set up mock Claude dir with only context7.
	// ---------------------------------------------------------------
	setupMockClaude(t, machine2Claude, []string{
		"context7@claude-plugins-official",
	})

	// ---------------------------------------------------------------
	// Step 6: Machine 2 — Join from the remote.
	// ---------------------------------------------------------------
	err = commands.Join(remoteDir, machine2Claude, machine2Sync)
	require.NoError(t, err, "Join should succeed on machine 2")

	// ---------------------------------------------------------------
	// Step 7: Machine 2 — Status should show beads in NotInstalled.
	// ---------------------------------------------------------------
	status2, err := commands.Status(machine2Claude, machine2Sync)
	require.NoError(t, err, "Status should succeed on machine 2")

	assert.Contains(t, status2.NotInstalled, "beads@beads-marketplace",
		"Machine 2 should report beads@beads-marketplace as not installed")

	// ---------------------------------------------------------------
	// Step 8: Machine 1 — Add new-plugin@local to mock Claude dir.
	// ---------------------------------------------------------------
	addPluginToMockClaude(t, machine1Claude, "new-plugin@local")

	// ---------------------------------------------------------------
	// Step 9: Machine 1 — PushApply to add the new plugin.
	// ---------------------------------------------------------------
	err = commands.PushApply(
		machine1Claude, machine1Sync,
		[]string{"new-plugin@local"}, nil,
		"Add new plugin",
	)
	require.NoError(t, err, "PushApply should succeed on machine 1")

	// ---------------------------------------------------------------
	// Step 10: Verify config.yaml now contains new-plugin@local.
	// ---------------------------------------------------------------
	cfgData, err := os.ReadFile(filepath.Join(machine1Sync, "config.yaml"))
	require.NoError(t, err, "should be able to read config.yaml after push")

	cfg, err := config.Parse(cfgData)
	require.NoError(t, err, "should be able to parse config.yaml after push")

	sort.Strings(cfg.Upstream)
	assert.Contains(t, cfg.Upstream, "new-plugin@local",
		"config.yaml should contain new-plugin@local after PushApply")
	assert.Contains(t, cfg.Upstream, "context7@claude-plugins-official",
		"config.yaml should still contain context7")
	assert.Contains(t, cfg.Upstream, "beads@beads-marketplace",
		"config.yaml should still contain beads")
	assert.Len(t, cfg.Upstream, 3, "config.yaml should have exactly 3 plugins")
}
