package commands_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/ruminaider/claude-sync/internal/claudemd"
	"github.com/ruminaider/claude-sync/internal/commands"
	"github.com/ruminaider/claude-sync/internal/config"
	"github.com/ruminaider/claude-sync/internal/profiles"
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

	assert.Equal(t, "1.0.0", cfg.Version)
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
	assert.Equal(t, "1.0.0", cfg.Version)
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

func TestInit_SkipsClaudeSyncForksEntries(t *testing.T) {
	claudeDir := t.TempDir()
	syncDir := filepath.Join(t.TempDir(), ".claude-sync")

	pluginDir := filepath.Join(claudeDir, "plugins")
	os.MkdirAll(pluginDir, 0755)

	// Simulate stale @claude-sync-forks entries from a previous init,
	// alongside the real upstream entries.
	plugins := `{
		"version": 2,
		"plugins": {
			"context7@claude-plugins-official": [{"scope":"user","installPath":"/p","version":"1.0","installedAt":"2026-01-01T00:00:00Z","lastUpdated":"2026-01-01T00:00:00Z"}],
			"context-anchor@context-anchor": [{"scope":"user","installPath":"/p","version":"1.2","installedAt":"2026-01-01T00:00:00Z","lastUpdated":"2026-01-01T00:00:00Z"}],
			"context-anchor@claude-sync-forks": [{"scope":"user","installPath":"/old/path","version":"1.2","installedAt":"2026-01-01T00:00:00Z","lastUpdated":"2026-01-01T00:00:00Z"}],
			"my-tool@claude-sync-forks": [{"scope":"user","installPath":"/old/path","version":"1.0","installedAt":"2026-01-01T00:00:00Z","lastUpdated":"2026-01-01T00:00:00Z"}]
		}
	}`
	os.WriteFile(filepath.Join(pluginDir, "installed_plugins.json"), []byte(plugins), 0644)

	// Mark context-anchor marketplace as github (portable).
	km := `{"context-anchor": {"source": {"source": "github", "repo": "ruminaider/context-anchor"}}}`
	os.WriteFile(filepath.Join(pluginDir, "known_marketplaces.json"), []byte(km), 0644)
	os.WriteFile(filepath.Join(claudeDir, "settings.json"), []byte("{}"), 0644)

	// Test InitScan.
	scan, err := commands.InitScan(claudeDir)
	require.NoError(t, err)

	assert.Contains(t, scan.Upstream, "context7@claude-plugins-official")
	assert.Contains(t, scan.Upstream, "context-anchor@context-anchor")
	// @claude-sync-forks entries should be completely absent.
	for _, key := range scan.PluginKeys {
		assert.NotContains(t, key, "claude-sync-forks")
	}
	for _, key := range scan.Upstream {
		assert.NotContains(t, key, "claude-sync-forks")
	}
	assert.Empty(t, scan.AutoForked)

	// Test Init.
	result, err := commands.Init(defaultInitOpts(claudeDir, syncDir, ""))
	require.NoError(t, err)

	assert.Contains(t, result.Upstream, "context7@claude-plugins-official")
	assert.Contains(t, result.Upstream, "context-anchor@context-anchor")
	assert.Empty(t, result.AutoForked)

	cfgData, _ := os.ReadFile(filepath.Join(syncDir, "config.yaml"))
	cfg, _ := config.Parse(cfgData)
	assert.Len(t, cfg.Upstream, 2)
	assert.Empty(t, cfg.Forked)
}

func TestInit_IncludesLocalScopePlugins(t *testing.T) {
	claudeDir := t.TempDir()
	syncDir := filepath.Join(t.TempDir(), ".claude-sync")

	pluginDir := filepath.Join(claudeDir, "plugins")
	os.MkdirAll(pluginDir, 0755)

	plugins := `{
		"version": 2,
		"plugins": {
			"context7@claude-plugins-official": [{"scope":"user","installPath":"/p","version":"1.0","installedAt":"2026-01-01T00:00:00Z","lastUpdated":"2026-01-01T00:00:00Z"}],
			"project-tool@some-marketplace": [{"scope":"local","installPath":"","projectPath":"/my/project","version":"1.0","installedAt":"2026-01-01T00:00:00Z","lastUpdated":"2026-01-01T00:00:00Z"}]
		}
	}`
	os.WriteFile(filepath.Join(pluginDir, "installed_plugins.json"), []byte(plugins), 0644)
	os.WriteFile(filepath.Join(pluginDir, "known_marketplaces.json"), []byte("{}"), 0644)
	os.WriteFile(filepath.Join(claudeDir, "settings.json"), []byte("{}"), 0644)

	result, err := commands.Init(defaultInitOpts(claudeDir, syncDir, ""))
	require.NoError(t, err)

	// Local-scope plugins are no longer skipped — they flow through normal classification.
	// project-tool has no installPath, so it falls through to upstream.
	assert.Contains(t, result.Upstream, "context7@claude-plugins-official")
	assert.Contains(t, result.Upstream, "project-tool@some-marketplace")
	assert.Empty(t, result.AutoForked)

	cfgData, _ := os.ReadFile(filepath.Join(syncDir, "config.yaml"))
	cfg, _ := config.Parse(cfgData)
	assert.Contains(t, cfg.Upstream, "context7@claude-plugins-official")
	assert.Contains(t, cfg.Upstream, "project-tool@some-marketplace")
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

// --- Plugin filtering tests ---

func TestInit_IncludesAllPluginsByDefault(t *testing.T) {
	claudeDir, syncDir := setupTestEnv(t)

	result, err := commands.Init(defaultInitOpts(claudeDir, syncDir, ""))
	require.NoError(t, err)

	// nil IncludePlugins = include all (backward compat).
	assert.Contains(t, result.Upstream, "context7@claude-plugins-official")
	assert.Contains(t, result.Upstream, "beads@beads-marketplace")
	assert.Empty(t, result.ExcludedPlugins)
}

func TestInit_FiltersPlugins(t *testing.T) {
	claudeDir, syncDir := setupTestEnv(t)

	result, err := commands.Init(commands.InitOptions{
		ClaudeDir:       claudeDir,
		SyncDir:         syncDir,
		IncludeSettings: true,
		IncludeHooks:    nil,
		IncludePlugins:  []string{"context7@claude-plugins-official"},
	})
	require.NoError(t, err)

	assert.Contains(t, result.Upstream, "context7@claude-plugins-official")
	assert.NotContains(t, result.Upstream, "beads@beads-marketplace")
	assert.Contains(t, result.ExcludedPlugins, "beads@beads-marketplace")

	cfgData, _ := os.ReadFile(filepath.Join(syncDir, "config.yaml"))
	cfg, _ := config.Parse(cfgData)
	assert.Contains(t, cfg.Upstream, "context7@claude-plugins-official")
	assert.NotContains(t, cfg.Upstream, "beads@beads-marketplace")
}

func TestInit_EmptyPluginList(t *testing.T) {
	claudeDir, syncDir := setupTestEnv(t)

	result, err := commands.Init(commands.InitOptions{
		ClaudeDir:       claudeDir,
		SyncDir:         syncDir,
		IncludeSettings: true,
		IncludeHooks:    nil,
		IncludePlugins:  []string{}, // empty = no plugins
	})
	require.NoError(t, err)

	assert.Empty(t, result.Upstream)
	assert.Empty(t, result.AutoForked)
	assert.Len(t, result.ExcludedPlugins, 2) // both plugins excluded

	cfgData, _ := os.ReadFile(filepath.Join(syncDir, "config.yaml"))
	cfg, _ := config.Parse(cfgData)
	assert.Empty(t, cfg.Upstream)
}

// --- Profile tests ---

func TestInit_CreatesProfiles(t *testing.T) {
	claudeDir, syncDir := setupTestEnv(t)

	workProfile := profiles.Profile{
		Settings: map[string]any{"model": "opus"},
		Plugins: profiles.ProfilePlugins{
			Add: []string{"extra-tool@some-marketplace"},
		},
	}
	personalProfile := profiles.Profile{
		Settings: map[string]any{"model": "sonnet"},
	}

	result, err := commands.Init(commands.InitOptions{
		ClaudeDir:       claudeDir,
		SyncDir:         syncDir,
		IncludeSettings: true,
		IncludeHooks:    nil,
		Profiles: map[string]profiles.Profile{
			"work":     workProfile,
			"personal": personalProfile,
		},
	})
	require.NoError(t, err)

	// ProfileNames should be sorted.
	assert.Equal(t, []string{"personal", "work"}, result.ProfileNames)

	// Verify profile files exist and are parseable.
	workData, err := os.ReadFile(filepath.Join(syncDir, "profiles", "work.yaml"))
	require.NoError(t, err)
	parsedWork, err := profiles.ParseProfile(workData)
	require.NoError(t, err)
	assert.Equal(t, "opus", parsedWork.Settings["model"])
	assert.Contains(t, parsedWork.Plugins.Add, "extra-tool@some-marketplace")

	personalData, err := os.ReadFile(filepath.Join(syncDir, "profiles", "personal.yaml"))
	require.NoError(t, err)
	parsedPersonal, err := profiles.ParseProfile(personalData)
	require.NoError(t, err)
	assert.Equal(t, "sonnet", parsedPersonal.Settings["model"])
}

func TestInit_ProfilesNil_NoProfileDir(t *testing.T) {
	claudeDir, syncDir := setupTestEnv(t)

	_, err := commands.Init(defaultInitOpts(claudeDir, syncDir, ""))
	require.NoError(t, err)

	// profiles/ directory should NOT exist when Profiles is nil.
	_, err = os.Stat(filepath.Join(syncDir, "profiles"))
	assert.True(t, os.IsNotExist(err), "profiles/ directory should not exist when Profiles is nil")
}

func TestInit_ActiveProfileWritten(t *testing.T) {
	claudeDir, syncDir := setupTestEnv(t)

	result, err := commands.Init(commands.InitOptions{
		ClaudeDir:       claudeDir,
		SyncDir:         syncDir,
		IncludeSettings: true,
		IncludeHooks:    nil,
		Profiles: map[string]profiles.Profile{
			"work": {Settings: map[string]any{"model": "opus"}},
		},
		ActiveProfile: "work",
	})
	require.NoError(t, err)

	assert.Equal(t, "work", result.ActiveProfile)

	// Verify active-profile file contains the right name.
	active, err := profiles.ReadActiveProfile(syncDir)
	require.NoError(t, err)
	assert.Equal(t, "work", active)
}

func TestInit_GitignoreIncludesActiveProfile(t *testing.T) {
	claudeDir, syncDir := setupTestEnv(t)

	_, err := commands.Init(defaultInitOpts(claudeDir, syncDir, ""))
	require.NoError(t, err)

	gitignore, err := os.ReadFile(filepath.Join(syncDir, ".gitignore"))
	require.NoError(t, err)
	assert.Contains(t, string(gitignore), "active-profile")
}

// --- New surface tests ---

func TestInitScan_Permissions(t *testing.T) {
	claudeDir := t.TempDir()
	pluginDir := filepath.Join(claudeDir, "plugins")
	os.MkdirAll(pluginDir, 0755)

	plugins := `{"version": 2, "plugins": {}}`
	os.WriteFile(filepath.Join(pluginDir, "installed_plugins.json"), []byte(plugins), 0644)
	os.WriteFile(filepath.Join(pluginDir, "known_marketplaces.json"), []byte("{}"), 0644)

	settings := `{
		"permissions": {
			"allow": ["Read", "Edit"],
			"deny": ["Bash"]
		}
	}`
	os.WriteFile(filepath.Join(claudeDir, "settings.json"), []byte(settings), 0644)

	scan, err := commands.InitScan(claudeDir)
	require.NoError(t, err)

	assert.Equal(t, []string{"Read", "Edit"}, scan.Permissions.Allow)
	assert.Equal(t, []string{"Bash"}, scan.Permissions.Deny)
	// Permissions should not appear in generic settings.
	_, hasPerms := scan.Settings["permissions"]
	assert.False(t, hasPerms)
}

func TestInitScan_ClaudeMD(t *testing.T) {
	claudeDir := t.TempDir()
	pluginDir := filepath.Join(claudeDir, "plugins")
	os.MkdirAll(pluginDir, 0755)

	plugins := `{"version": 2, "plugins": {}}`
	os.WriteFile(filepath.Join(pluginDir, "installed_plugins.json"), []byte(plugins), 0644)
	os.WriteFile(filepath.Join(pluginDir, "known_marketplaces.json"), []byte("{}"), 0644)
	os.WriteFile(filepath.Join(claudeDir, "settings.json"), []byte("{}"), 0644)

	claudeMD := "# My Config\n\n## Section One\nSome content\n\n## Section Two\nMore content\n"
	os.WriteFile(filepath.Join(claudeDir, "CLAUDE.md"), []byte(claudeMD), 0644)

	scan, err := commands.InitScan(claudeDir)
	require.NoError(t, err)

	assert.Equal(t, claudeMD, scan.ClaudeMDContent)
}

func TestInitScan_MCP(t *testing.T) {
	claudeDir := t.TempDir()
	pluginDir := filepath.Join(claudeDir, "plugins")
	os.MkdirAll(pluginDir, 0755)

	plugins := `{"version": 2, "plugins": {}}`
	os.WriteFile(filepath.Join(pluginDir, "installed_plugins.json"), []byte(plugins), 0644)
	os.WriteFile(filepath.Join(pluginDir, "known_marketplaces.json"), []byte("{}"), 0644)
	os.WriteFile(filepath.Join(claudeDir, "settings.json"), []byte("{}"), 0644)

	mcpConfig := `{
		"mcpServers": {
			"context7": {"command": "npx", "args": ["-y", "@context7/mcp"]},
			"memory": {"command": "npx", "args": ["-y", "@memory/mcp"]}
		}
	}`
	os.WriteFile(filepath.Join(claudeDir, ".mcp.json"), []byte(mcpConfig), 0644)

	scan, err := commands.InitScan(claudeDir)
	require.NoError(t, err)

	assert.Len(t, scan.MCP, 2)
	assert.Contains(t, scan.MCP, "context7")
	assert.Contains(t, scan.MCP, "memory")
}

func TestInitScan_Keybindings(t *testing.T) {
	claudeDir := t.TempDir()
	pluginDir := filepath.Join(claudeDir, "plugins")
	os.MkdirAll(pluginDir, 0755)

	plugins := `{"version": 2, "plugins": {}}`
	os.WriteFile(filepath.Join(pluginDir, "installed_plugins.json"), []byte(plugins), 0644)
	os.WriteFile(filepath.Join(pluginDir, "known_marketplaces.json"), []byte("{}"), 0644)
	os.WriteFile(filepath.Join(claudeDir, "settings.json"), []byte("{}"), 0644)

	kb := `{"ctrl+k": "clear", "ctrl+l": "log"}`
	os.WriteFile(filepath.Join(claudeDir, "keybindings.json"), []byte(kb), 0644)

	scan, err := commands.InitScan(claudeDir)
	require.NoError(t, err)

	assert.Len(t, scan.Keybindings, 2)
	assert.Equal(t, "clear", scan.Keybindings["ctrl+k"])
	assert.Equal(t, "log", scan.Keybindings["ctrl+l"])
}

func TestInitScan_GenericSettings(t *testing.T) {
	claudeDir := t.TempDir()
	pluginDir := filepath.Join(claudeDir, "plugins")
	os.MkdirAll(pluginDir, 0755)

	plugins := `{"version": 2, "plugins": {}}`
	os.WriteFile(filepath.Join(pluginDir, "installed_plugins.json"), []byte(plugins), 0644)
	os.WriteFile(filepath.Join(pluginDir, "known_marketplaces.json"), []byte("{}"), 0644)

	settings := `{
		"model": "claude-sonnet-4-5-20250929",
		"statusLine": "fancy",
		"theme": "dark",
		"enabledPlugins": {"beads@beads-marketplace": true},
		"hooks": {"PreCompact": [{"matcher":"","hooks":[{"type":"command","command":"bd prime"}]}]},
		"permissions": {"allow": ["Read"]}
	}`
	os.WriteFile(filepath.Join(claudeDir, "settings.json"), []byte(settings), 0644)

	scan, err := commands.InitScan(claudeDir)
	require.NoError(t, err)

	// model, statusLine, theme should be in generic settings.
	assert.Equal(t, "claude-sonnet-4-5-20250929", scan.Settings["model"])
	assert.Equal(t, "fancy", scan.Settings["statusLine"])
	assert.Equal(t, "dark", scan.Settings["theme"])

	// enabledPlugins, hooks, permissions should NOT be in generic settings.
	_, hasEnabled := scan.Settings["enabledPlugins"]
	assert.False(t, hasEnabled)
	_, hasHooks := scan.Settings["hooks"]
	assert.False(t, hasHooks)
	_, hasPerms := scan.Settings["permissions"]
	assert.False(t, hasPerms)

	// Hooks and permissions should be in their dedicated fields.
	assertHookHasCommand(t, scan.Hooks["PreCompact"], "bd prime")
	assert.Equal(t, []string{"Read"}, scan.Permissions.Allow)
}

func TestInit_WithClaudeMD(t *testing.T) {
	claudeDir := t.TempDir()
	syncDir := filepath.Join(t.TempDir(), ".claude-sync")
	pluginDir := filepath.Join(claudeDir, "plugins")
	os.MkdirAll(pluginDir, 0755)

	plugins := `{"version": 2, "plugins": {}}`
	os.WriteFile(filepath.Join(pluginDir, "installed_plugins.json"), []byte(plugins), 0644)
	os.WriteFile(filepath.Join(pluginDir, "known_marketplaces.json"), []byte("{}"), 0644)
	os.WriteFile(filepath.Join(claudeDir, "settings.json"), []byte("{}"), 0644)

	claudeMD := "## Git Commits\nNo co-authored-by\n\n## Color Schemes\nUse Catppuccin Mocha\n"
	os.WriteFile(filepath.Join(claudeDir, "CLAUDE.md"), []byte(claudeMD), 0644)

	result, err := commands.Init(commands.InitOptions{
		ClaudeDir:      claudeDir,
		SyncDir:        syncDir,
		IncludeSettings: true,
		IncludeHooks:   nil,
		ImportClaudeMD: true,
	})
	require.NoError(t, err)

	assert.Len(t, result.ClaudeMDFragments, 2)
	assert.Contains(t, result.ClaudeMDFragments, "git-commits")
	assert.Contains(t, result.ClaudeMDFragments, "color-schemes")

	// Verify fragment files exist.
	_, err = os.Stat(filepath.Join(syncDir, "claude-md", "git-commits.md"))
	assert.NoError(t, err)
	_, err = os.Stat(filepath.Join(syncDir, "claude-md", "color-schemes.md"))
	assert.NoError(t, err)

	// Verify config.yaml has claude_md.include.
	cfgData, _ := os.ReadFile(filepath.Join(syncDir, "config.yaml"))
	cfg, err := config.Parse(cfgData)
	require.NoError(t, err)
	assert.Equal(t, []string{"git-commits", "color-schemes"}, cfg.ClaudeMD.Include)
}

func TestInit_WithPermissions(t *testing.T) {
	claudeDir := t.TempDir()
	syncDir := filepath.Join(t.TempDir(), ".claude-sync")
	pluginDir := filepath.Join(claudeDir, "plugins")
	os.MkdirAll(pluginDir, 0755)

	plugins := `{"version": 2, "plugins": {}}`
	os.WriteFile(filepath.Join(pluginDir, "installed_plugins.json"), []byte(plugins), 0644)
	os.WriteFile(filepath.Join(pluginDir, "known_marketplaces.json"), []byte("{}"), 0644)
	os.WriteFile(filepath.Join(claudeDir, "settings.json"), []byte("{}"), 0644)

	result, err := commands.Init(commands.InitOptions{
		ClaudeDir:       claudeDir,
		SyncDir:         syncDir,
		IncludeSettings: true,
		IncludeHooks:    nil,
		Permissions: config.Permissions{
			Allow: []string{"Read", "Edit"},
			Deny:  []string{"Bash"},
		},
	})
	require.NoError(t, err)
	assert.True(t, result.PermissionsIncluded)

	cfgData, _ := os.ReadFile(filepath.Join(syncDir, "config.yaml"))
	cfg, err := config.Parse(cfgData)
	require.NoError(t, err)
	assert.Equal(t, []string{"Read", "Edit"}, cfg.Permissions.Allow)
	assert.Equal(t, []string{"Bash"}, cfg.Permissions.Deny)
}

func TestInit_WithMCP(t *testing.T) {
	claudeDir := t.TempDir()
	syncDir := filepath.Join(t.TempDir(), ".claude-sync")
	pluginDir := filepath.Join(claudeDir, "plugins")
	os.MkdirAll(pluginDir, 0755)

	plugins := `{"version": 2, "plugins": {}}`
	os.WriteFile(filepath.Join(pluginDir, "installed_plugins.json"), []byte(plugins), 0644)
	os.WriteFile(filepath.Join(pluginDir, "known_marketplaces.json"), []byte("{}"), 0644)
	os.WriteFile(filepath.Join(claudeDir, "settings.json"), []byte("{}"), 0644)

	mcp := map[string]json.RawMessage{
		"context7": json.RawMessage(`{"command":"npx","args":["-y","@context7/mcp"]}`),
	}

	result, err := commands.Init(commands.InitOptions{
		ClaudeDir:       claudeDir,
		SyncDir:         syncDir,
		IncludeSettings: true,
		IncludeHooks:    nil,
		MCP:             mcp,
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"context7"}, result.MCPIncluded)

	cfgData, _ := os.ReadFile(filepath.Join(syncDir, "config.yaml"))
	cfg, err := config.Parse(cfgData)
	require.NoError(t, err)
	assert.Contains(t, cfg.MCP, "context7")
}

func TestInit_WithKeybindings(t *testing.T) {
	claudeDir := t.TempDir()
	syncDir := filepath.Join(t.TempDir(), ".claude-sync")
	pluginDir := filepath.Join(claudeDir, "plugins")
	os.MkdirAll(pluginDir, 0755)

	plugins := `{"version": 2, "plugins": {}}`
	os.WriteFile(filepath.Join(pluginDir, "installed_plugins.json"), []byte(plugins), 0644)
	os.WriteFile(filepath.Join(pluginDir, "known_marketplaces.json"), []byte("{}"), 0644)
	os.WriteFile(filepath.Join(claudeDir, "settings.json"), []byte("{}"), 0644)

	result, err := commands.Init(commands.InitOptions{
		ClaudeDir:       claudeDir,
		SyncDir:         syncDir,
		IncludeSettings: true,
		IncludeHooks:    nil,
		Keybindings:     map[string]any{"ctrl+k": "clear"},
	})
	require.NoError(t, err)
	assert.True(t, result.KeybindingsIncluded)

	cfgData, _ := os.ReadFile(filepath.Join(syncDir, "config.yaml"))
	cfg, err := config.Parse(cfgData)
	require.NoError(t, err)
	assert.Equal(t, "clear", cfg.Keybindings["ctrl+k"])
}

func TestInit_GitignoreHasPendingChanges(t *testing.T) {
	claudeDir, syncDir := setupTestEnv(t)

	_, err := commands.Init(defaultInitOpts(claudeDir, syncDir, ""))
	require.NoError(t, err)

	gitignore, err := os.ReadFile(filepath.Join(syncDir, ".gitignore"))
	require.NoError(t, err)
	assert.Contains(t, string(gitignore), "pending-changes.yaml")
}

func TestInit_SettingsFilter(t *testing.T) {
	claudeDir := t.TempDir()
	syncDir := filepath.Join(t.TempDir(), ".claude-sync")
	pluginDir := filepath.Join(claudeDir, "plugins")
	os.MkdirAll(pluginDir, 0755)

	plugins := `{"version": 2, "plugins": {}}`
	os.WriteFile(filepath.Join(pluginDir, "installed_plugins.json"), []byte(plugins), 0644)
	os.WriteFile(filepath.Join(pluginDir, "known_marketplaces.json"), []byte("{}"), 0644)
	os.WriteFile(filepath.Join(claudeDir, "settings.json"), []byte(`{"model": "opus", "statusLine": "fancy", "theme": "dark"}`), 0644)

	result, err := commands.Init(commands.InitOptions{
		ClaudeDir:       claudeDir,
		SyncDir:         syncDir,
		IncludeSettings: true,
		SettingsFilter:  []string{"model"},
	})
	require.NoError(t, err)

	assert.Equal(t, []string{"model"}, result.IncludedSettings)

	cfgData, err := os.ReadFile(filepath.Join(syncDir, "config.yaml"))
	require.NoError(t, err)
	cfg, err := config.Parse(cfgData)
	require.NoError(t, err)

	_, hasModel := cfg.Settings["model"]
	assert.True(t, hasModel)
	_, hasStatusLine := cfg.Settings["statusLine"]
	assert.False(t, hasStatusLine)
	_, hasTheme := cfg.Settings["theme"]
	assert.False(t, hasTheme)
}

func TestInit_ClaudeMDFragmentFilter(t *testing.T) {
	claudeDir := t.TempDir()
	syncDir := filepath.Join(t.TempDir(), ".claude-sync")
	pluginDir := filepath.Join(claudeDir, "plugins")
	os.MkdirAll(pluginDir, 0755)

	plugins := `{"version": 2, "plugins": {}}`
	os.WriteFile(filepath.Join(pluginDir, "installed_plugins.json"), []byte(plugins), 0644)
	os.WriteFile(filepath.Join(pluginDir, "known_marketplaces.json"), []byte("{}"), 0644)
	os.WriteFile(filepath.Join(claudeDir, "settings.json"), []byte("{}"), 0644)

	claudeMD := "## Git Commits\nNo co-authored-by\n\n## Color Schemes\nUse Catppuccin Mocha\n"
	os.WriteFile(filepath.Join(claudeDir, "CLAUDE.md"), []byte(claudeMD), 0644)

	result, err := commands.Init(commands.InitOptions{
		ClaudeDir:         claudeDir,
		SyncDir:           syncDir,
		IncludeSettings:   true,
		ImportClaudeMD:    true,
		ClaudeMDFragments: []string{"git-commits"},
	})
	require.NoError(t, err)

	assert.Equal(t, []string{"git-commits"}, result.ClaudeMDFragments)

	// Both fragment files should exist on disk (all are imported to disk).
	_, err = os.Stat(filepath.Join(syncDir, "claude-md", "git-commits.md"))
	assert.NoError(t, err)
	_, err = os.Stat(filepath.Join(syncDir, "claude-md", "color-schemes.md"))
	assert.NoError(t, err)

	// Only the selected fragment should be in config.yaml include list.
	cfgData, err := os.ReadFile(filepath.Join(syncDir, "config.yaml"))
	require.NoError(t, err)
	cfg, err := config.Parse(cfgData)
	require.NoError(t, err)
	assert.Equal(t, []string{"git-commits"}, cfg.ClaudeMD.Include)
}

func TestInitScan_ClaudeMDSections(t *testing.T) {
	claudeDir := t.TempDir()
	pluginDir := filepath.Join(claudeDir, "plugins")
	os.MkdirAll(pluginDir, 0755)

	plugins := `{"version": 2, "plugins": {}}`
	os.WriteFile(filepath.Join(pluginDir, "installed_plugins.json"), []byte(plugins), 0644)
	os.WriteFile(filepath.Join(pluginDir, "known_marketplaces.json"), []byte("{}"), 0644)
	os.WriteFile(filepath.Join(claudeDir, "settings.json"), []byte("{}"), 0644)

	content := "# My Config\n\n## Section One\nSome content\n\n## Section Two\nMore content\n"
	os.WriteFile(filepath.Join(claudeDir, "CLAUDE.md"), []byte(content), 0644)

	scan, err := commands.InitScan(claudeDir)
	require.NoError(t, err)

	assert.NotNil(t, scan.ClaudeMDSections)

	expectedSections := claudemd.Split(content)
	assert.Equal(t, len(expectedSections), len(scan.ClaudeMDSections))

	for i, sec := range scan.ClaudeMDSections {
		assert.Equal(t, expectedSections[i].Header, sec.Header)
	}
}

func TestInit_PermissionsFiltered(t *testing.T) {
	claudeDir := t.TempDir()
	syncDir := filepath.Join(t.TempDir(), ".claude-sync")
	pluginDir := filepath.Join(claudeDir, "plugins")
	os.MkdirAll(pluginDir, 0755)

	plugins := `{"version": 2, "plugins": {}}`
	os.WriteFile(filepath.Join(pluginDir, "installed_plugins.json"), []byte(plugins), 0644)
	os.WriteFile(filepath.Join(pluginDir, "known_marketplaces.json"), []byte("{}"), 0644)
	os.WriteFile(filepath.Join(claudeDir, "settings.json"), []byte("{}"), 0644)

	result, err := commands.Init(commands.InitOptions{
		ClaudeDir:       claudeDir,
		SyncDir:         syncDir,
		IncludeSettings: true,
		Permissions:     config.Permissions{Allow: []string{"Read"}},
	})
	require.NoError(t, err)

	assert.True(t, result.PermissionsIncluded)

	cfgData, err := os.ReadFile(filepath.Join(syncDir, "config.yaml"))
	require.NoError(t, err)
	cfg, err := config.Parse(cfgData)
	require.NoError(t, err)
	assert.Equal(t, []string{"Read"}, cfg.Permissions.Allow)
	assert.Empty(t, cfg.Permissions.Deny)
}

func TestInit_MCPFiltered(t *testing.T) {
	claudeDir := t.TempDir()
	syncDir := filepath.Join(t.TempDir(), ".claude-sync")
	pluginDir := filepath.Join(claudeDir, "plugins")
	os.MkdirAll(pluginDir, 0755)

	plugins := `{"version": 2, "plugins": {}}`
	os.WriteFile(filepath.Join(pluginDir, "installed_plugins.json"), []byte(plugins), 0644)
	os.WriteFile(filepath.Join(pluginDir, "known_marketplaces.json"), []byte("{}"), 0644)
	os.WriteFile(filepath.Join(claudeDir, "settings.json"), []byte("{}"), 0644)

	// Only pass "context7" to Init, not "memory".
	mcp := map[string]json.RawMessage{
		"context7": json.RawMessage(`{"command":"npx","args":["-y","@context7/mcp"]}`),
	}

	result, err := commands.Init(commands.InitOptions{
		ClaudeDir:       claudeDir,
		SyncDir:         syncDir,
		IncludeSettings: true,
		MCP:             mcp,
	})
	require.NoError(t, err)

	assert.Equal(t, []string{"context7"}, result.MCPIncluded)

	cfgData, err := os.ReadFile(filepath.Join(syncDir, "config.yaml"))
	require.NoError(t, err)
	cfg, err := config.Parse(cfgData)
	require.NoError(t, err)
	assert.Contains(t, cfg.MCP, "context7")
	_, hasMemory := cfg.MCP["memory"]
	assert.False(t, hasMemory)
}
