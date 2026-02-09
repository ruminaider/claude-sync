package commands_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/ruminaider/claude-sync/internal/claudecode"
	"github.com/ruminaider/claude-sync/internal/commands"
	"github.com/ruminaider/claude-sync/internal/config"
	"github.com/ruminaider/claude-sync/internal/plugins"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPull_DryRun_NothingToInstall(t *testing.T) {
	claudeDir, syncDir := setupStatusEnv(t)

	result, err := commands.PullDryRun(claudeDir, syncDir)
	require.NoError(t, err)
	assert.Empty(t, result.ToInstall)
}

func TestPull_NoSyncDir(t *testing.T) {
	_, err := commands.PullDryRun("/tmp/test-claude", "/nonexistent")
	assert.Error(t, err)
}

func TestPull_GitPull(t *testing.T) {
	remote := t.TempDir()
	exec.Command("git", "init", "--bare", remote).Run()

	claudeDir, syncDir := setupStatusEnv(t)
	exec.Command("git", "-C", syncDir, "remote", "add", "origin", remote).Run()
	exec.Command("git", "-C", syncDir, "push", "-u", "origin", "master").Run()

	result, err := commands.PullDryRun(claudeDir, syncDir)
	require.NoError(t, err)
	assert.Empty(t, result.ToInstall)
}

func TestPull_RespectsUserPreferences(t *testing.T) {
	claudeDir, syncDir := setupStatusEnv(t)

	prefs := "sync_mode: union\nplugins:\n  unsubscribe:\n    - beads@beads-marketplace\n"
	os.WriteFile(filepath.Join(syncDir, "user-preferences.yaml"), []byte(prefs), 0644)

	result, err := commands.PullDryRun(claudeDir, syncDir)
	require.NoError(t, err)
	assert.NotContains(t, result.EffectiveDesired, "beads@beads-marketplace")
}

func TestPull_ExactMode(t *testing.T) {
	claudeDir, syncDir := setupStatusEnv(t)

	// Add extra plugin to installed
	pluginsPath := filepath.Join(claudeDir, "plugins", "installed_plugins.json")
	data := `{
		"version": 2,
		"plugins": {
			"context7@claude-plugins-official": [{"scope":"user","installPath":"/p","version":"1.0","installedAt":"2026-01-01T00:00:00Z","lastUpdated":"2026-01-01T00:00:00Z"}],
			"beads@beads-marketplace": [{"scope":"user","installPath":"/p","version":"0.44","installedAt":"2026-01-01T00:00:00Z","lastUpdated":"2026-01-01T00:00:00Z"}],
			"extra@local": [{"scope":"user","installPath":"/p","version":"1.0","installedAt":"2026-01-01T00:00:00Z","lastUpdated":"2026-01-01T00:00:00Z"}]
		}
	}`
	os.WriteFile(pluginsPath, []byte(data), 0644)

	// Set exact mode
	prefs := "sync_mode: exact\n"
	os.WriteFile(filepath.Join(syncDir, "user-preferences.yaml"), []byte(prefs), 0644)

	result, err := commands.PullDryRun(claudeDir, syncDir)
	require.NoError(t, err)
	assert.Contains(t, result.ToRemove, "extra@local")
	assert.Empty(t, result.Untracked)
}

// setupV2PullEnv creates a test environment with a v2 config that includes a forked plugin.
// It returns (claudeDir, syncDir) where syncDir contains a git repo with a v2 config.yaml
// listing a forked plugin, and a forked plugin directory with a valid manifest.
func setupV2PullEnv(t *testing.T) (claudeDir, syncDir string) {
	t.Helper()

	// Create claudeDir with plugins dir, installed_plugins.json, known_marketplaces.json
	claudeDir = t.TempDir()
	err := claudecode.Bootstrap(claudeDir)
	require.NoError(t, err)

	// Create syncDir
	syncDir = filepath.Join(t.TempDir(), ".claude-sync")
	require.NoError(t, os.MkdirAll(syncDir, 0755))

	// Write a v2 config.yaml with upstream and forked plugins
	configYAML := `version: "2.0.0"
plugins:
  upstream:
    - context7@claude-plugins-official
  forked:
    - my-custom-tool
`
	require.NoError(t, os.WriteFile(filepath.Join(syncDir, "config.yaml"), []byte(configYAML), 0644))

	// Create the forked plugin directory with a valid manifest
	pluginDir := filepath.Join(syncDir, "plugins", "my-custom-tool", ".claude-plugin")
	require.NoError(t, os.MkdirAll(pluginDir, 0755))
	manifest := `{"name": "my-custom-tool", "version": "1.0.0"}`
	require.NoError(t, os.WriteFile(filepath.Join(pluginDir, "plugin.json"), []byte(manifest), 0644))

	// Initialize a git repo in syncDir
	require.NoError(t, exec.Command("git", "init", syncDir).Run())
	require.NoError(t, exec.Command("git", "-C", syncDir, "config", "user.email", "test@test.com").Run())
	require.NoError(t, exec.Command("git", "-C", syncDir, "config", "user.name", "Test").Run())
	require.NoError(t, exec.Command("git", "-C", syncDir, "add", ".").Run())
	require.NoError(t, exec.Command("git", "-C", syncDir, "commit", "-m", "init").Run())

	return claudeDir, syncDir
}

func TestPull_RegistersLocalMarketplace(t *testing.T) {
	claudeDir, syncDir := setupV2PullEnv(t)

	result, err := commands.PullDryRun(claudeDir, syncDir)
	require.NoError(t, err)

	// Verify known_marketplaces.json contains "claude-sync-forks" entry
	mkts, err := claudecode.ReadMarketplaces(claudeDir)
	require.NoError(t, err)
	assert.Contains(t, mkts, plugins.MarketplaceName)

	// Parse the entry to verify structure
	var entry struct {
		Source struct {
			Source string `json:"source"`
			Path   string `json:"path"`
		} `json:"source"`
	}
	err = json.Unmarshal(mkts[plugins.MarketplaceName], &entry)
	require.NoError(t, err)
	assert.Equal(t, "directory", entry.Source.Source)
	assert.Equal(t, filepath.Join(syncDir, "plugins"), entry.Source.Path)

	// Verify forked plugin key appears in effective desired list
	assert.Contains(t, result.EffectiveDesired, "my-custom-tool@claude-sync-forks")
	// Verify upstream plugin is also present
	assert.Contains(t, result.EffectiveDesired, "context7@claude-plugins-official")
	// Forked plugin should be in ToInstall since it's not installed yet
	assert.Contains(t, result.ToInstall, "my-custom-tool@claude-sync-forks")
}

// --- ApplySettings tests ---

func setupApplySettingsEnv(t *testing.T) string {
	t.Helper()
	claudeDir := t.TempDir()
	os.MkdirAll(filepath.Join(claudeDir, "plugins"), 0755)
	return claudeDir
}

func readSettingsJSON(t *testing.T, claudeDir string) map[string]json.RawMessage {
	t.Helper()
	settings, err := claudecode.ReadSettings(claudeDir)
	require.NoError(t, err)
	return settings
}

func TestApplySettings_Model(t *testing.T) {
	claudeDir := setupApplySettingsEnv(t)

	cfg := config.Config{
		Settings: map[string]any{"model": "claude-sonnet-4-5-20250929"},
	}

	applied, hooks, err := commands.ApplySettings(claudeDir, cfg)
	require.NoError(t, err)
	assert.Equal(t, []string{"model"}, applied)
	assert.Empty(t, hooks)

	settings := readSettingsJSON(t, claudeDir)
	var model string
	require.NoError(t, json.Unmarshal(settings["model"], &model))
	assert.Equal(t, "claude-sonnet-4-5-20250929", model)
}

func TestApplySettings_HooksExpanded(t *testing.T) {
	claudeDir := setupApplySettingsEnv(t)

	cfg := config.Config{
		Hooks: map[string]json.RawMessage{
			"PreCompact":   config.ExpandHookCommand("bd prime"),
			"SessionStart": config.ExpandHookCommand("bd prime"),
		},
	}

	applied, hooks, err := commands.ApplySettings(claudeDir, cfg)
	require.NoError(t, err)
	assert.Empty(t, applied)
	assert.Len(t, hooks, 2)
	assert.Contains(t, hooks, "PreCompact")
	assert.Contains(t, hooks, "SessionStart")

	// Verify the expanded hook structure in settings.json.
	settings := readSettingsJSON(t, claudeDir)
	var hooksMap map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(settings["hooks"], &hooksMap))

	// Each hook should have the full nested structure.
	for _, hookName := range []string{"PreCompact", "SessionStart"} {
		var entries []struct {
			Matcher string `json:"matcher"`
			Hooks   []struct {
				Type    string `json:"type"`
				Command string `json:"command"`
			} `json:"hooks"`
		}
		require.NoError(t, json.Unmarshal(hooksMap[hookName], &entries))
		require.Len(t, entries, 1)
		assert.Equal(t, "", entries[0].Matcher)
		require.Len(t, entries[0].Hooks, 1)
		assert.Equal(t, "command", entries[0].Hooks[0].Type)
		assert.Equal(t, "bd prime", entries[0].Hooks[0].Command)
	}
}

func TestApplySettings_ExcludedFieldsPreserved(t *testing.T) {
	claudeDir := setupApplySettingsEnv(t)

	// Write settings.json with excluded fields.
	existing := map[string]json.RawMessage{
		"enabledPlugins": json.RawMessage(`{"beads@beads-marketplace": true}`),
		"permissions":    json.RawMessage(`{"allow": ["Read"]}`),
		"statusLine":     json.RawMessage(`"compact"`),
		"model":          json.RawMessage(`"old-model"`),
	}
	require.NoError(t, claudecode.WriteSettings(claudeDir, existing))

	// Config tries to overwrite excluded fields — they should be ignored.
	cfg := config.Config{
		Settings: map[string]any{
			"model":          "new-model",
			"enabledPlugins": map[string]bool{"evil": true},
			"permissions":    "bad",
		},
	}

	applied, _, err := commands.ApplySettings(claudeDir, cfg)
	require.NoError(t, err)
	assert.Equal(t, []string{"model"}, applied)

	settings := readSettingsJSON(t, claudeDir)

	// Model should be updated.
	var model string
	require.NoError(t, json.Unmarshal(settings["model"], &model))
	assert.Equal(t, "new-model", model)

	// Excluded fields should be untouched.
	assert.JSONEq(t, `{"beads@beads-marketplace": true}`, string(settings["enabledPlugins"]))
	assert.JSONEq(t, `{"allow": ["Read"]}`, string(settings["permissions"]))
	assert.Equal(t, `"compact"`, string(settings["statusLine"]))
}

func TestApplySettings_PreservesLocalSettings(t *testing.T) {
	claudeDir := setupApplySettingsEnv(t)

	// Existing settings.json with local-only settings and a local hook.
	existing := map[string]json.RawMessage{
		"theme": json.RawMessage(`"dark"`),
		"hooks": json.RawMessage(`{"PostToolUse":[{"matcher":"","hooks":[{"type":"command","command":"local-hook"}]}]}`),
	}
	require.NoError(t, claudecode.WriteSettings(claudeDir, existing))

	cfg := config.Config{
		Settings: map[string]any{"model": "claude-sonnet-4-5-20250929"},
		Hooks:    map[string]json.RawMessage{"PreCompact": config.ExpandHookCommand("bd prime")},
	}

	applied, hooks, err := commands.ApplySettings(claudeDir, cfg)
	require.NoError(t, err)
	assert.Equal(t, []string{"model"}, applied)
	assert.Equal(t, []string{"PreCompact"}, hooks)

	settings := readSettingsJSON(t, claudeDir)

	// Local "theme" setting should be preserved.
	assert.Equal(t, `"dark"`, string(settings["theme"]))

	// Local "PostToolUse" hook should be preserved alongside new "PreCompact".
	var hooksMap map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(settings["hooks"], &hooksMap))
	assert.Contains(t, hooksMap, "PostToolUse")
	assert.Contains(t, hooksMap, "PreCompact")
}

func TestApplySettings_NoopWhenEmpty(t *testing.T) {
	claudeDir := setupApplySettingsEnv(t)

	cfg := config.Config{} // no settings, no hooks

	applied, hooks, err := commands.ApplySettings(claudeDir, cfg)
	require.NoError(t, err)
	assert.Nil(t, applied)
	assert.Nil(t, hooks)

	// settings.json should not have been created.
	_, err = os.Stat(filepath.Join(claudeDir, "settings.json"))
	assert.True(t, os.IsNotExist(err))
}

func TestApplySettings_CreatesSettingsIfMissing(t *testing.T) {
	claudeDir := setupApplySettingsEnv(t)
	// No settings.json exists.

	cfg := config.Config{
		Settings: map[string]any{"model": "claude-sonnet-4-5-20250929"},
		Hooks:    map[string]json.RawMessage{"SessionStart": config.ExpandHookCommand("bd prime")},
	}

	applied, hooks, err := commands.ApplySettings(claudeDir, cfg)
	require.NoError(t, err)
	assert.Equal(t, []string{"model"}, applied)
	assert.Equal(t, []string{"SessionStart"}, hooks)

	// settings.json should now exist with the right content.
	settings := readSettingsJSON(t, claudeDir)
	var model string
	require.NoError(t, json.Unmarshal(settings["model"], &model))
	assert.Equal(t, "claude-sonnet-4-5-20250929", model)
	assert.Contains(t, settings, "hooks")
}

// setupPullEnvWithSettingsAndHooks creates a sync dir with settings and hooks in config.yaml,
// plus a claudeDir with matching installed plugins.
func setupPullEnvWithSettingsAndHooks(t *testing.T) (claudeDir, syncDir string) {
	t.Helper()

	claudeDir = t.TempDir()
	err := claudecode.Bootstrap(claudeDir)
	require.NoError(t, err)

	syncDir = filepath.Join(t.TempDir(), ".claude-sync")
	require.NoError(t, os.MkdirAll(syncDir, 0755))

	configYAML := `version: "2.0.0"
plugins:
  upstream:
    - context7@claude-plugins-official
settings:
  model: opus
hooks:
  PreCompact: "bd prime"
  SessionStart: "pull --quiet"
`
	require.NoError(t, os.WriteFile(filepath.Join(syncDir, "config.yaml"), []byte(configYAML), 0644))

	require.NoError(t, exec.Command("git", "init", syncDir).Run())
	require.NoError(t, exec.Command("git", "-C", syncDir, "config", "user.email", "test@test.com").Run())
	require.NoError(t, exec.Command("git", "-C", syncDir, "config", "user.name", "Test").Run())
	require.NoError(t, exec.Command("git", "-C", syncDir, "add", ".").Run())
	require.NoError(t, exec.Command("git", "-C", syncDir, "commit", "-m", "init").Run())

	return claudeDir, syncDir
}

func TestPull_SkipsHooksWhenPrefsSet(t *testing.T) {
	claudeDir, syncDir := setupPullEnvWithSettingsAndHooks(t)

	// Write user-preferences with hooks skipped.
	prefs := "sync_mode: union\nsync:\n  skip:\n    - hooks\n"
	os.WriteFile(filepath.Join(syncDir, "user-preferences.yaml"), []byte(prefs), 0644)

	result, err := commands.Pull(claudeDir, syncDir, true)
	require.NoError(t, err)

	assert.Contains(t, result.SkippedCategories, "hooks")
	assert.Empty(t, result.HooksApplied)
	// Settings should still be applied.
	assert.Contains(t, result.SettingsApplied, "model")
}

func TestPull_SkipsSettingsWhenPrefsSet(t *testing.T) {
	claudeDir, syncDir := setupPullEnvWithSettingsAndHooks(t)

	// Write user-preferences with settings skipped.
	prefs := "sync_mode: union\nsync:\n  skip:\n    - settings\n"
	os.WriteFile(filepath.Join(syncDir, "user-preferences.yaml"), []byte(prefs), 0644)

	result, err := commands.Pull(claudeDir, syncDir, true)
	require.NoError(t, err)

	assert.Contains(t, result.SkippedCategories, "settings")
	assert.Empty(t, result.SettingsApplied)
	// Hooks should still be applied.
	assert.NotEmpty(t, result.HooksApplied)
}

func TestPull_SkipsBothWhenPrefsSet(t *testing.T) {
	claudeDir, syncDir := setupPullEnvWithSettingsAndHooks(t)

	prefs := "sync_mode: union\nsync:\n  skip:\n    - settings\n    - hooks\n"
	os.WriteFile(filepath.Join(syncDir, "user-preferences.yaml"), []byte(prefs), 0644)

	result, err := commands.Pull(claudeDir, syncDir, true)
	require.NoError(t, err)

	assert.Len(t, result.SkippedCategories, 2)
	assert.Empty(t, result.SettingsApplied)
	assert.Empty(t, result.HooksApplied)
}

func TestApplySettings_SyncedHookOverridesLocal(t *testing.T) {
	claudeDir := setupApplySettingsEnv(t)

	// Existing hook for PreCompact with a different command.
	existing := map[string]json.RawMessage{
		"hooks": json.RawMessage(`{"PreCompact":[{"matcher":"","hooks":[{"type":"command","command":"old-command"}]}]}`),
	}
	require.NoError(t, claudecode.WriteSettings(claudeDir, existing))

	cfg := config.Config{
		Hooks: map[string]json.RawMessage{"PreCompact": config.ExpandHookCommand("new-command")},
	}

	_, hooks, err := commands.ApplySettings(claudeDir, cfg)
	require.NoError(t, err)
	assert.Equal(t, []string{"PreCompact"}, hooks)

	settings := readSettingsJSON(t, claudeDir)
	var hooksMap map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(settings["hooks"], &hooksMap))

	var entries []struct {
		Hooks []struct {
			Command string `json:"command"`
		} `json:"hooks"`
	}
	require.NoError(t, json.Unmarshal(hooksMap["PreCompact"], &entries))
	assert.Equal(t, "new-command", entries[0].Hooks[0].Command)
}
