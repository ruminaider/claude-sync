package commands_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/ruminaider/claude-sync/internal/commands"
	"github.com/ruminaider/claude-sync/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// makeHookJSON builds a json.RawMessage for a single-command hook.
func makeHookJSON(command string) json.RawMessage {
	return config.ExpandHookCommand(command)
}

// assertHookHasCommand checks that a json.RawMessage hook entry contains the expected command.
func assertHookHasCommand(t *testing.T, hookData json.RawMessage, expectedCmd string) {
	t.Helper()
	cmd := commands.ExtractHookCommand(hookData)
	assert.Equal(t, expectedCmd, cmd)
}

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

// defaultInitOpts builds InitOptions with backward-compatible defaults (include all).
func defaultInitOpts(claudeDir, syncDir, remoteURL string) commands.InitOptions {
	return commands.InitOptions{
		ClaudeDir:       claudeDir,
		SyncDir:         syncDir,
		RemoteURL:       remoteURL,
		IncludeSettings: true,
		IncludeHooks:    nil, // nil = include all hooks
	}
}

func TestInit(t *testing.T) {
	claudeDir, syncDir := setupTestEnv(t)

	result, err := commands.Init(defaultInitOpts(claudeDir, syncDir, ""))
	require.NoError(t, err)
	require.NotNil(t, result)

	cfgData, err := os.ReadFile(filepath.Join(syncDir, "config.yaml"))
	require.NoError(t, err)

	cfg, err := config.Parse(cfgData)
	require.NoError(t, err)

	assert.Equal(t, "2.1.0", cfg.Version)
	assert.Contains(t, cfg.Upstream, "context7@claude-plugins-official")
	assert.Contains(t, cfg.Upstream, "beads@beads-marketplace")
}

func TestInit_AlreadyExists(t *testing.T) {
	claudeDir, syncDir := setupTestEnv(t)
	os.MkdirAll(syncDir, 0755)

	_, err := commands.Init(defaultInitOpts(claudeDir, syncDir, ""))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already exists")
}

func TestInit_NoClaudeDir(t *testing.T) {
	syncDir := filepath.Join(t.TempDir(), ".claude-sync")
	_, err := commands.Init(defaultInitOpts("/nonexistent", syncDir, ""))
	assert.Error(t, err)
}

func TestInit_ExtractsHooks(t *testing.T) {
	claudeDir, syncDir := setupTestEnv(t)

	_, err := commands.Init(defaultInitOpts(claudeDir, syncDir, ""))
	require.NoError(t, err)

	cfgData, _ := os.ReadFile(filepath.Join(syncDir, "config.yaml"))
	cfg, _ := config.Parse(cfgData)
	assertHookHasCommand(t, cfg.Hooks["PreCompact"], "bd prime")
}

func TestInit_FiltersSettings(t *testing.T) {
	claudeDir, syncDir := setupTestEnv(t)

	_, err := commands.Init(defaultInitOpts(claudeDir, syncDir, ""))
	require.NoError(t, err)

	cfgData, _ := os.ReadFile(filepath.Join(syncDir, "config.yaml"))
	cfg, _ := config.Parse(cfgData)

	_, hasEnabled := cfg.Settings["enabledPlugins"]
	assert.False(t, hasEnabled)
}

func TestInit_CreatesGitRepo(t *testing.T) {
	claudeDir, syncDir := setupTestEnv(t)

	_, err := commands.Init(defaultInitOpts(claudeDir, syncDir, ""))
	require.NoError(t, err)

	_, err = os.Stat(filepath.Join(syncDir, ".git"))
	assert.NoError(t, err)
}

func TestInit_GitignoresUserPreferences(t *testing.T) {
	claudeDir, syncDir := setupTestEnv(t)

	_, err := commands.Init(defaultInitOpts(claudeDir, syncDir, ""))
	require.NoError(t, err)

	gitignore, err := os.ReadFile(filepath.Join(syncDir, ".gitignore"))
	require.NoError(t, err)
	assert.Contains(t, string(gitignore), "user-preferences.yaml")
}

func TestInit_CreatesV2Config(t *testing.T) {
	claudeDir, syncDir := setupTestEnv(t)
	_, err := commands.Init(defaultInitOpts(claudeDir, syncDir, ""))
	require.NoError(t, err)

	data, _ := os.ReadFile(filepath.Join(syncDir, "config.yaml"))
	cfg, err := config.Parse(data)
	require.NoError(t, err)
	assert.Equal(t, "2.1.0", cfg.Version)
	assert.NotEmpty(t, cfg.Upstream)
	assert.Empty(t, cfg.Pinned)
	assert.Empty(t, cfg.Forked)
}

func TestInit_AutoForksNonPortablePlugins(t *testing.T) {
	claudeDir := t.TempDir()
	syncDir := filepath.Join(t.TempDir(), ".claude-sync")

	pluginDir := filepath.Join(claudeDir, "plugins")
	os.MkdirAll(pluginDir, 0755)

	// Create a real install path with a plugin.json for the non-portable plugin.
	customInstallDir := filepath.Join(t.TempDir(), "custom-plugin-files")
	os.MkdirAll(customInstallDir, 0755)
	os.WriteFile(filepath.Join(customInstallDir, "plugin.json"), []byte(`{"name":"my-tool","version":"1.0"}`), 0644)
	os.WriteFile(filepath.Join(customInstallDir, "index.js"), []byte(`console.log("hello")`), 0644)

	plugins := `{
		"version": 2,
		"plugins": {
			"context7@claude-plugins-official": [{"scope":"user","installPath":"/p","version":"1.0","installedAt":"2026-01-01T00:00:00Z","lastUpdated":"2026-01-01T00:00:00Z"}],
			"my-tool@local-custom-plugins": [{"scope":"user","installPath":"` + customInstallDir + `","version":"1.0","installedAt":"2026-01-01T00:00:00Z","lastUpdated":"2026-01-01T00:00:00Z"}]
		}
	}`
	os.WriteFile(filepath.Join(pluginDir, "installed_plugins.json"), []byte(plugins), 0644)
	os.WriteFile(filepath.Join(pluginDir, "known_marketplaces.json"), []byte("{}"), 0644)
	os.WriteFile(filepath.Join(claudeDir, "settings.json"), []byte("{}"), 0644)

	result, err := commands.Init(defaultInitOpts(claudeDir, syncDir, ""))
	require.NoError(t, err)

	// context7 should be upstream; my-tool should be auto-forked.
	assert.Contains(t, result.Upstream, "context7@claude-plugins-official")
	assert.Contains(t, result.AutoForked, "my-tool@local-custom-plugins")
	assert.Empty(t, result.Skipped)

	// Config should reflect the categorization.
	cfgData, _ := os.ReadFile(filepath.Join(syncDir, "config.yaml"))
	cfg, err := config.Parse(cfgData)
	require.NoError(t, err)
	assert.Contains(t, cfg.Upstream, "context7@claude-plugins-official")
	assert.NotContains(t, cfg.Upstream, "my-tool@local-custom-plugins")
	assert.Contains(t, cfg.Forked, "my-tool")

	// Plugin files should have been copied.
	_, err = os.Stat(filepath.Join(syncDir, "plugins", "my-tool", "plugin.json"))
	assert.NoError(t, err)
	_, err = os.Stat(filepath.Join(syncDir, "plugins", "my-tool", "index.js"))
	assert.NoError(t, err)
}

func TestInit_SkipsLocalScopePlugins(t *testing.T) {
	claudeDir := t.TempDir()
	syncDir := filepath.Join(t.TempDir(), ".claude-sync")

	pluginDir := filepath.Join(claudeDir, "plugins")
	os.MkdirAll(pluginDir, 0755)

	plugins := `{
		"version": 2,
		"plugins": {
			"context7@claude-plugins-official": [{"scope":"user","installPath":"/p","version":"1.0","installedAt":"2026-01-01T00:00:00Z","lastUpdated":"2026-01-01T00:00:00Z"}],
			"project-tool@some-marketplace": [{"scope":"local","installPath":"/p","projectPath":"/my/project","version":"1.0","installedAt":"2026-01-01T00:00:00Z","lastUpdated":"2026-01-01T00:00:00Z"}]
		}
	}`
	os.WriteFile(filepath.Join(pluginDir, "installed_plugins.json"), []byte(plugins), 0644)
	os.WriteFile(filepath.Join(pluginDir, "known_marketplaces.json"), []byte("{}"), 0644)
	os.WriteFile(filepath.Join(claudeDir, "settings.json"), []byte("{}"), 0644)

	result, err := commands.Init(defaultInitOpts(claudeDir, syncDir, ""))
	require.NoError(t, err)

	assert.Contains(t, result.Upstream, "context7@claude-plugins-official")
	assert.Contains(t, result.Skipped, "project-tool@some-marketplace")
	assert.Empty(t, result.AutoForked)

	// Config should only have the upstream plugin.
	cfgData, _ := os.ReadFile(filepath.Join(syncDir, "config.yaml"))
	cfg, _ := config.Parse(cfgData)
	assert.Contains(t, cfg.Upstream, "context7@claude-plugins-official")
	assert.NotContains(t, cfg.Upstream, "project-tool@some-marketplace")
}

func TestInit_AllPortableStaysUpstream(t *testing.T) {
	claudeDir := t.TempDir()
	syncDir := filepath.Join(t.TempDir(), ".claude-sync")

	pluginDir := filepath.Join(claudeDir, "plugins")
	os.MkdirAll(pluginDir, 0755)

	plugins := `{
		"version": 2,
		"plugins": {
			"context7@claude-plugins-official": [{"scope":"user","installPath":"/p","version":"1.0","installedAt":"2026-01-01T00:00:00Z","lastUpdated":"2026-01-01T00:00:00Z"}],
			"beads@beads-marketplace": [{"scope":"user","installPath":"/p","version":"0.44","installedAt":"2026-01-01T00:00:00Z","lastUpdated":"2026-01-01T00:00:00Z"}],
			"superpowers@superpowers-marketplace": [{"scope":"user","installPath":"/p","version":"2.0","installedAt":"2026-01-01T00:00:00Z","lastUpdated":"2026-01-01T00:00:00Z"}]
		}
	}`
	os.WriteFile(filepath.Join(pluginDir, "installed_plugins.json"), []byte(plugins), 0644)
	os.WriteFile(filepath.Join(pluginDir, "known_marketplaces.json"), []byte("{}"), 0644)
	os.WriteFile(filepath.Join(claudeDir, "settings.json"), []byte("{}"), 0644)

	result, err := commands.Init(defaultInitOpts(claudeDir, syncDir, ""))
	require.NoError(t, err)

	assert.Len(t, result.Upstream, 3)
	assert.Empty(t, result.AutoForked)
	assert.Empty(t, result.Skipped)

	cfgData, _ := os.ReadFile(filepath.Join(syncDir, "config.yaml"))
	cfg, _ := config.Parse(cfgData)
	assert.Len(t, cfg.Upstream, 3)
	assert.Empty(t, cfg.Forked)
}

// --- New tests for InitScan and granular Init ---

func TestInitScan(t *testing.T) {
	claudeDir, _ := setupTestEnv(t)

	scan, err := commands.InitScan(claudeDir)
	require.NoError(t, err)

	assert.Contains(t, scan.Upstream, "context7@claude-plugins-official")
	assert.Contains(t, scan.Upstream, "beads@beads-marketplace")
	assertHookHasCommand(t, scan.Hooks["PreCompact"], "bd prime")
	assert.Empty(t, scan.Settings) // no model in test settings
}

func TestInitScan_WithModel(t *testing.T) {
	claudeDir := t.TempDir()
	pluginDir := filepath.Join(claudeDir, "plugins")
	os.MkdirAll(pluginDir, 0755)

	plugins := `{"version": 2, "plugins": {}}`
	os.WriteFile(filepath.Join(pluginDir, "installed_plugins.json"), []byte(plugins), 0644)
	os.WriteFile(filepath.Join(pluginDir, "known_marketplaces.json"), []byte("{}"), 0644)

	settings := `{"model": "claude-sonnet-4-5-20250929", "hooks": {"SessionStart": [{"matcher":"","hooks":[{"type":"command","command":"bd prime"}]}]}}`
	os.WriteFile(filepath.Join(claudeDir, "settings.json"), []byte(settings), 0644)

	scan, err := commands.InitScan(claudeDir)
	require.NoError(t, err)

	assert.Equal(t, "claude-sonnet-4-5-20250929", scan.Settings["model"])
	assertHookHasCommand(t, scan.Hooks["SessionStart"], "bd prime")
}

func TestInit_ExcludesSettings(t *testing.T) {
	claudeDir := t.TempDir()
	syncDir := filepath.Join(t.TempDir(), ".claude-sync")
	pluginDir := filepath.Join(claudeDir, "plugins")
	os.MkdirAll(pluginDir, 0755)

	plugins := `{"version": 2, "plugins": {}}`
	os.WriteFile(filepath.Join(pluginDir, "installed_plugins.json"), []byte(plugins), 0644)
	os.WriteFile(filepath.Join(pluginDir, "known_marketplaces.json"), []byte("{}"), 0644)
	os.WriteFile(filepath.Join(claudeDir, "settings.json"), []byte(`{"model": "opus"}`), 0644)

	result, err := commands.Init(commands.InitOptions{
		ClaudeDir:       claudeDir,
		SyncDir:         syncDir,
		IncludeSettings: false,
		IncludeHooks:    nil,
	})
	require.NoError(t, err)
	assert.Empty(t, result.IncludedSettings)

	cfgData, _ := os.ReadFile(filepath.Join(syncDir, "config.yaml"))
	cfg, _ := config.Parse(cfgData)
	assert.Empty(t, cfg.Settings)
}

func TestInit_ExcludesHooks(t *testing.T) {
	claudeDir, syncDir := setupTestEnv(t)

	result, err := commands.Init(commands.InitOptions{
		ClaudeDir:       claudeDir,
		SyncDir:         syncDir,
		IncludeSettings: true,
		IncludeHooks:    map[string]json.RawMessage{}, // empty = no hooks
	})
	require.NoError(t, err)
	assert.Empty(t, result.IncludedHooks)

	cfgData, _ := os.ReadFile(filepath.Join(syncDir, "config.yaml"))
	cfg, _ := config.Parse(cfgData)
	assert.Empty(t, cfg.Hooks)
}

func TestInit_PartialHooks(t *testing.T) {
	claudeDir := t.TempDir()
	syncDir := filepath.Join(t.TempDir(), ".claude-sync")
	pluginDir := filepath.Join(claudeDir, "plugins")
	os.MkdirAll(pluginDir, 0755)

	plugins := `{"version": 2, "plugins": {}}`
	os.WriteFile(filepath.Join(pluginDir, "installed_plugins.json"), []byte(plugins), 0644)
	os.WriteFile(filepath.Join(pluginDir, "known_marketplaces.json"), []byte("{}"), 0644)

	settings := `{"hooks": {"PreCompact": [{"matcher":"","hooks":[{"type":"command","command":"bd prime"}]}], "SessionStart": [{"matcher":"","hooks":[{"type":"command","command":"pull"}]}]}}`
	os.WriteFile(filepath.Join(claudeDir, "settings.json"), []byte(settings), 0644)

	result, err := commands.Init(commands.InitOptions{
		ClaudeDir:       claudeDir,
		SyncDir:         syncDir,
		IncludeSettings: true,
		IncludeHooks:    map[string]json.RawMessage{"PreCompact": makeHookJSON("bd prime")}, // only PreCompact
	})
	require.NoError(t, err)

	assert.Equal(t, []string{"PreCompact"}, result.IncludedHooks)

	cfgData, _ := os.ReadFile(filepath.Join(syncDir, "config.yaml"))
	cfg, _ := config.Parse(cfgData)
	assertHookHasCommand(t, cfg.Hooks["PreCompact"], "bd prime")
	_, hasSessionStart := cfg.Hooks["SessionStart"]
	assert.False(t, hasSessionStart)
}

func TestInit_IncludedSettingsAndHooksReported(t *testing.T) {
	claudeDir := t.TempDir()
	syncDir := filepath.Join(t.TempDir(), ".claude-sync")
	pluginDir := filepath.Join(claudeDir, "plugins")
	os.MkdirAll(pluginDir, 0755)

	plugins := `{"version": 2, "plugins": {}}`
	os.WriteFile(filepath.Join(pluginDir, "installed_plugins.json"), []byte(plugins), 0644)
	os.WriteFile(filepath.Join(pluginDir, "known_marketplaces.json"), []byte("{}"), 0644)
	os.WriteFile(filepath.Join(claudeDir, "settings.json"), []byte(`{"model": "opus", "hooks": {"PreCompact": [{"matcher":"","hooks":[{"type":"command","command":"bd prime"}]}]}}`), 0644)

	result, err := commands.Init(commands.InitOptions{
		ClaudeDir:       claudeDir,
		SyncDir:         syncDir,
		IncludeSettings: true,
		IncludeHooks:    nil, // all hooks
	})
	require.NoError(t, err)

	assert.Equal(t, []string{"model"}, result.IncludedSettings)
	assert.Equal(t, []string{"PreCompact"}, result.IncludedHooks)

	// Verify sorted
	sort.Strings(result.IncludedSettings)
	sort.Strings(result.IncludedHooks)
}
