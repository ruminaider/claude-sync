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

func setupTestEnv(t *testing.T) (claudeDir, syncDir string) {
	t.Helper()
	claudeDir = t.TempDir()
	syncDir = filepath.Join(t.TempDir(), ".claude-sync")

	pluginDir := filepath.Join(claudeDir, "plugins")
	os.MkdirAll(pluginDir, 0755)

	plugins := `{
		"version": 2,
		"plugins": {
			"context7@claude-plugins-official": [{"scope":"user","installPath":"/p","version":"1.0","installedAt":"2026-01-01T00:00:00Z","lastUpdated":"2026-01-01T00:00:00Z"}],
			"beads@beads-marketplace": [{"scope":"user","installPath":"/p","version":"0.44","installedAt":"2026-01-01T00:00:00Z","lastUpdated":"2026-01-01T00:00:00Z"}]
		}
	}`
	os.WriteFile(filepath.Join(pluginDir, "installed_plugins.json"), []byte(plugins), 0644)
	os.WriteFile(filepath.Join(pluginDir, "known_marketplaces.json"), []byte("{}"), 0644)

	settings := `{
		"hooks": {
			"PreCompact": [{"matcher":"","hooks":[{"type":"command","command":"bd prime"}]}]
		},
		"enabledPlugins": {"beads@beads-marketplace": true}
	}`
	os.WriteFile(filepath.Join(claudeDir, "settings.json"), []byte(settings), 0644)

	return claudeDir, syncDir
}

func TestInit(t *testing.T) {
	claudeDir, syncDir := setupTestEnv(t)

	err := commands.Init(claudeDir, syncDir)
	require.NoError(t, err)

	cfgData, err := os.ReadFile(filepath.Join(syncDir, "config.yaml"))
	require.NoError(t, err)

	cfg, err := config.Parse(cfgData)
	require.NoError(t, err)

	assert.Equal(t, "1.0.0", cfg.Version)
	assert.Contains(t, cfg.Upstream, "context7@claude-plugins-official")
	assert.Contains(t, cfg.Upstream, "beads@beads-marketplace")
}

func TestInit_AlreadyExists(t *testing.T) {
	claudeDir, syncDir := setupTestEnv(t)
	os.MkdirAll(syncDir, 0755)

	err := commands.Init(claudeDir, syncDir)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already exists")
}

func TestInit_NoClaudeDir(t *testing.T) {
	syncDir := filepath.Join(t.TempDir(), ".claude-sync")
	err := commands.Init("/nonexistent", syncDir)
	assert.Error(t, err)
}

func TestInit_ExtractsHooks(t *testing.T) {
	claudeDir, syncDir := setupTestEnv(t)

	err := commands.Init(claudeDir, syncDir)
	require.NoError(t, err)

	cfgData, _ := os.ReadFile(filepath.Join(syncDir, "config.yaml"))
	cfg, _ := config.Parse(cfgData)
	assert.Equal(t, "bd prime", cfg.Hooks["PreCompact"])
}

func TestInit_FiltersSettings(t *testing.T) {
	claudeDir, syncDir := setupTestEnv(t)

	err := commands.Init(claudeDir, syncDir)
	require.NoError(t, err)

	cfgData, _ := os.ReadFile(filepath.Join(syncDir, "config.yaml"))
	cfg, _ := config.Parse(cfgData)

	_, hasEnabled := cfg.Settings["enabledPlugins"]
	assert.False(t, hasEnabled)
}

func TestInit_CreatesGitRepo(t *testing.T) {
	claudeDir, syncDir := setupTestEnv(t)

	err := commands.Init(claudeDir, syncDir)
	require.NoError(t, err)

	_, err = os.Stat(filepath.Join(syncDir, ".git"))
	assert.NoError(t, err)
}

func TestInit_GitignoresUserPreferences(t *testing.T) {
	claudeDir, syncDir := setupTestEnv(t)

	err := commands.Init(claudeDir, syncDir)
	require.NoError(t, err)

	gitignore, err := os.ReadFile(filepath.Join(syncDir, ".gitignore"))
	require.NoError(t, err)
	assert.Contains(t, string(gitignore), "user-preferences.yaml")
}
