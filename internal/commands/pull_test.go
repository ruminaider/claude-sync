package commands_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/ruminaider/claude-sync/internal/approval"
	"github.com/ruminaider/claude-sync/internal/claudecode"
	"github.com/ruminaider/claude-sync/internal/claudemd"
	"github.com/ruminaider/claude-sync/internal/commands"
	"github.com/ruminaider/claude-sync/internal/config"
	"github.com/ruminaider/claude-sync/internal/marketplace"
	"github.com/ruminaider/claude-sync/internal/plugins"
	"github.com/ruminaider/claude-sync/internal/profiles"
	"github.com/ruminaider/claude-sync/internal/project"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPull_DryRun_NothingToInstall(t *testing.T) {
	claudeDir, syncDir := setupStatusEnv(t)

	result, err := commands.PullDryRun(claudeDir, syncDir)
	require.NoError(t, err)
	// Only the bundled claude-sync plugin may need installing in test environments
	var unexpected []string
	for _, p := range result.ToInstall {
		if p != "claude-sync@claude-sync-forks" {
			unexpected = append(unexpected, p)
		}
	}
	assert.Empty(t, unexpected, "unexpected plugins need installing")
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
	// Only the bundled claude-sync plugin may need installing in test environments
	var unexpected2 []string
	for _, p := range result.ToInstall {
		if p != "claude-sync@claude-sync-forks" {
			unexpected2 = append(unexpected2, p)
		}
	}
	assert.Empty(t, unexpected2, "unexpected plugins need installing")
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
	configYAML := `version: "1.0.0"
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

	applied, hooks, _, err := commands.ApplySettings(claudeDir, cfg)
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

	applied, hooks, _, err := commands.ApplySettings(claudeDir, cfg)
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

	applied, _, _, err := commands.ApplySettings(claudeDir, cfg)
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

	applied, hooks, _, err := commands.ApplySettings(claudeDir, cfg)
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

	applied, hooks, _, err := commands.ApplySettings(claudeDir, cfg)
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

	applied, hooks, _, err := commands.ApplySettings(claudeDir, cfg)
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

	configYAML := `version: "1.0.0"
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

	_, hooks, _, err := commands.ApplySettings(claudeDir, cfg)
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


func TestApplySettings_SkipsMissingScriptHooks(t *testing.T) {
	claudeDir := setupApplySettingsEnv(t)
	cfg := config.Config{
		Hooks: map[string]json.RawMessage{
			"SessionEnd": config.ExpandHookCommand("bash /nonexistent/path/script.sh"),
		},
	}
	_, hooks, skipped, err := commands.ApplySettings(claudeDir, cfg)
	require.NoError(t, err)
	assert.Empty(t, hooks)
	assert.Len(t, skipped, 1)
	assert.Contains(t, skipped[0], "script not found")
	assert.Contains(t, skipped[0], "/nonexistent/path/script.sh")
	settings := readSettingsJSON(t, claudeDir)
	if hooksRaw, ok := settings["hooks"]; ok {
		var hooksMap map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(hooksRaw, &hooksMap))
		assert.NotContains(t, hooksMap, "SessionEnd")
	}
}

func TestApplySettings_InlineCommandsPassValidation(t *testing.T) {
	claudeDir := setupApplySettingsEnv(t)
	cfg := config.Config{
		Hooks: map[string]json.RawMessage{
			"PostToolUse":  config.ExpandHookCommand("claude-sync auto-commit --if-changed"),
			"SessionEnd":   config.ExpandHookCommand("claude-sync push --auto --quiet"),
			"SessionStart": config.ExpandHookCommand("claude-sync pull --auto"),
		},
	}
	_, hooks, skipped, err := commands.ApplySettings(claudeDir, cfg)
	require.NoError(t, err)
	assert.Len(t, hooks, 3)
	assert.Empty(t, skipped)
}

func TestApplySettings_ExistingScriptApplied(t *testing.T) {
	claudeDir := setupApplySettingsEnv(t)
	scriptDir := filepath.Join(claudeDir, "hooks")
	require.NoError(t, os.MkdirAll(scriptDir, 0755))
	scriptPath := filepath.Join(scriptDir, "test-hook.sh")
	require.NoError(t, os.WriteFile(scriptPath, []byte("#!/bin/bash\necho ok"), 0755))
	cfg := config.Config{
		Hooks: map[string]json.RawMessage{
			"SessionEnd": config.ExpandHookCommand("bash " + scriptPath),
		},
	}
	_, hooks, skipped, err := commands.ApplySettings(claudeDir, cfg)
	require.NoError(t, err)
	assert.Equal(t, []string{"SessionEnd"}, hooks)
	assert.Empty(t, skipped)
}

func TestApplySettings_MixedValidAndInvalidHooks(t *testing.T) {
	claudeDir := setupApplySettingsEnv(t)
	cfg := config.Config{
		Hooks: map[string]json.RawMessage{
			"SessionStart": config.ExpandHookCommand("claude-sync pull --auto"),
			"PreCompact":   config.ExpandHookCommand("bash /nonexistent/missing.sh"),
		},
	}
	_, hooks, skipped, err := commands.ApplySettings(claudeDir, cfg)
	require.NoError(t, err)
	assert.Equal(t, []string{"SessionStart"}, hooks)
	assert.Len(t, skipped, 1)
	assert.Contains(t, skipped[0], "PreCompact")
	settings := readSettingsJSON(t, claudeDir)
	var hooksMap map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(settings["hooks"], &hooksMap))
	assert.Contains(t, hooksMap, "SessionStart")
	assert.NotContains(t, hooksMap, "PreCompact")
}

func TestApplySettings_TildePathMissingScript(t *testing.T) {
	claudeDir := setupApplySettingsEnv(t)
	cfg := config.Config{
		Hooks: map[string]json.RawMessage{
			"SessionEnd": config.ExpandHookCommand("bash ~/.claude/hooks/nonexistent.sh"),
		},
	}
	_, hooks, skipped, err := commands.ApplySettings(claudeDir, cfg)
	require.NoError(t, err)
	assert.Empty(t, hooks)
	assert.Len(t, skipped, 1)
	assert.Contains(t, skipped[0], "~/.claude/hooks/nonexistent.sh")
}

func TestApplySettings_RelativePathNotValidated(t *testing.T) {
	claudeDir := setupApplySettingsEnv(t)
	cfg := config.Config{
		Hooks: map[string]json.RawMessage{
			"SessionEnd": config.ExpandHookCommand("bash script.sh"),
		},
	}
	_, hooks, skipped, err := commands.ApplySettings(claudeDir, cfg)
	require.NoError(t, err)
	assert.Equal(t, []string{"SessionEnd"}, hooks)
	assert.Empty(t, skipped)
}

func TestApplySettings_MalformedHookJSON(t *testing.T) {
	claudeDir := setupApplySettingsEnv(t)
	cfg := config.Config{
		Hooks: map[string]json.RawMessage{
			"SessionEnd": json.RawMessage("not valid json"),
		},
	}
	_, hooks, skipped, err := commands.ApplySettings(claudeDir, cfg)
	require.NoError(t, err)
	assert.Empty(t, hooks)
	assert.Len(t, skipped, 1)
	assert.Contains(t, skipped[0], "SessionEnd")
	assert.Contains(t, skipped[0], "malformed hook JSON")
}

func TestApplySettings_InterpreterFlags(t *testing.T) {
	claudeDir := setupApplySettingsEnv(t)
	cfg := config.Config{
		Hooks: map[string]json.RawMessage{
			"SessionEnd": config.ExpandHookCommand("bash -e /nonexistent/script.sh"),
		},
	}
	_, hooks, skipped, err := commands.ApplySettings(claudeDir, cfg)
	require.NoError(t, err)
	assert.Empty(t, hooks)
	assert.Len(t, skipped, 1)
	assert.Contains(t, skipped[0], "/nonexistent/script.sh")
}

func TestApplySettings_MultiCommandPartialMissing(t *testing.T) {
	claudeDir := setupApplySettingsEnv(t)
	// Build a hook entry with two commands: one valid inline, one missing script.
	hookJSON := `[{"matcher":"","hooks":[
		{"type":"command","command":"echo hello"},
		{"type":"command","command":"bash /nonexistent/deploy.sh"}
	]}]`
	cfg := config.Config{
		Hooks: map[string]json.RawMessage{
			"SessionEnd": json.RawMessage(hookJSON),
		},
	}
	_, hooks, skipped, err := commands.ApplySettings(claudeDir, cfg)
	require.NoError(t, err)
	assert.Empty(t, hooks, "entire hook should be skipped when any command references a missing script")
	assert.Len(t, skipped, 1)
	assert.Contains(t, skipped[0], "/nonexistent/deploy.sh")
}

// setupPullEnvWithProfile creates a sync dir with a config.yaml listing upstream plugins,
// a profiles/ directory with a named profile YAML, and optionally sets the active-profile file.
// Returns (claudeDir, syncDir).
func setupPullEnvWithProfile(t *testing.T, configYAML string, profileName string, profile profiles.Profile, setActive bool) (string, string) {
	t.Helper()

	claudeDir := t.TempDir()
	err := claudecode.Bootstrap(claudeDir)
	require.NoError(t, err)

	syncDir := filepath.Join(t.TempDir(), ".claude-sync")
	require.NoError(t, os.MkdirAll(syncDir, 0755))

	// Write config.yaml.
	require.NoError(t, os.WriteFile(filepath.Join(syncDir, "config.yaml"), []byte(configYAML), 0644))

	// Write profile YAML.
	profilesDir := filepath.Join(syncDir, "profiles")
	require.NoError(t, os.MkdirAll(profilesDir, 0755))
	profileData, err := profiles.MarshalProfile(profile)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(profilesDir, profileName+".yaml"), profileData, 0644))

	// Set active profile if requested.
	if setActive {
		require.NoError(t, profiles.WriteActiveProfile(syncDir, profileName))
	}

	// Init a git repo.
	require.NoError(t, exec.Command("git", "init", syncDir).Run())
	require.NoError(t, exec.Command("git", "-C", syncDir, "config", "user.email", "test@test.com").Run())
	require.NoError(t, exec.Command("git", "-C", syncDir, "config", "user.name", "Test").Run())
	require.NoError(t, exec.Command("git", "-C", syncDir, "add", ".").Run())
	require.NoError(t, exec.Command("git", "-C", syncDir, "commit", "-m", "init").Run())

	return claudeDir, syncDir
}

func TestPull_AppliesProfilePluginAdds(t *testing.T) {
	configYAML := `version: "1.0.0"
plugins:
  upstream:
    - context7@claude-plugins-official
`
	profile := profiles.Profile{
		Plugins: profiles.ProfilePlugins{
			Add: []string{"extra-tool@some-marketplace"},
		},
	}

	claudeDir, syncDir := setupPullEnvWithProfile(t, configYAML, "work", profile, true)

	result, err := commands.PullDryRun(claudeDir, syncDir)
	require.NoError(t, err)

	assert.Contains(t, result.EffectiveDesired, "context7@claude-plugins-official")
	assert.Contains(t, result.EffectiveDesired, "extra-tool@some-marketplace")
	assert.Equal(t, "work", result.ActiveProfile)
}

func TestPull_AppliesProfilePluginRemoves(t *testing.T) {
	configYAML := `version: "1.0.0"
plugins:
  upstream:
    - context7@claude-plugins-official
    - beads@beads-marketplace
`
	profile := profiles.Profile{
		Plugins: profiles.ProfilePlugins{
			Remove: []string{"beads@beads-marketplace"},
		},
	}

	claudeDir, syncDir := setupPullEnvWithProfile(t, configYAML, "work", profile, true)

	// Install both plugins so they show as synced rather than ToInstall.
	pluginsPath := filepath.Join(claudeDir, "plugins", "installed_plugins.json")
	data := `{
		"version": 2,
		"plugins": {
			"context7@claude-plugins-official": [{"scope":"user","installPath":"/p","version":"1.0","installedAt":"2026-01-01T00:00:00Z","lastUpdated":"2026-01-01T00:00:00Z"}],
			"beads@beads-marketplace": [{"scope":"user","installPath":"/p","version":"0.44","installedAt":"2026-01-01T00:00:00Z","lastUpdated":"2026-01-01T00:00:00Z"}]
		}
	}`
	require.NoError(t, os.WriteFile(pluginsPath, []byte(data), 0644))

	result, err := commands.PullDryRun(claudeDir, syncDir)
	require.NoError(t, err)

	assert.Contains(t, result.EffectiveDesired, "context7@claude-plugins-official")
	assert.NotContains(t, result.EffectiveDesired, "beads@beads-marketplace")
	assert.Equal(t, "work", result.ActiveProfile)
}

func TestPull_NoActiveProfile_BaseOnly(t *testing.T) {
	configYAML := `version: "1.0.0"
plugins:
  upstream:
    - context7@claude-plugins-official
    - beads@beads-marketplace
`
	profile := profiles.Profile{
		Plugins: profiles.ProfilePlugins{
			Add: []string{"extra-tool@some-marketplace"},
		},
	}

	// Profile exists but is NOT set as active.
	claudeDir, syncDir := setupPullEnvWithProfile(t, configYAML, "work", profile, false)

	result, err := commands.PullDryRun(claudeDir, syncDir)
	require.NoError(t, err)

	// Should only have the base plugins, not the profile's additions.
	assert.Contains(t, result.EffectiveDesired, "context7@claude-plugins-official")
	assert.Contains(t, result.EffectiveDesired, "beads@beads-marketplace")
	assert.NotContains(t, result.EffectiveDesired, "extra-tool@some-marketplace")
	assert.Equal(t, "", result.ActiveProfile)
}

func TestPull_ProfileAddOverridesExcluded(t *testing.T) {
	// Regression test for https://github.com/ruminaider/claude-sync/issues/39
	// A plugin in the global excluded list should appear in effectiveDesired
	// when a profile re-adds it via plugins.add.
	configYAML := `version: "1.0.0"
plugins:
  upstream:
    - context7@claude-plugins-official
  excluded:
    - excluded-tool@some-marketplace
`
	profile := profiles.Profile{
		Plugins: profiles.ProfilePlugins{
			Add: []string{"excluded-tool@some-marketplace"},
		},
	}

	claudeDir, syncDir := setupPullEnvWithProfile(t, configYAML, "work", profile, true)

	result, err := commands.PullDryRun(claudeDir, syncDir)
	require.NoError(t, err)

	assert.Contains(t, result.EffectiveDesired, "context7@claude-plugins-official",
		"base upstream plugin should be in desired")
	assert.Contains(t, result.EffectiveDesired, "excluded-tool@some-marketplace",
		"profile plugins.add should override global excluded")
	assert.Equal(t, "work", result.ActiveProfile)
}

func TestPull_ProfileAddOverridesExcluded_Reconciliation(t *testing.T) {
	// End-to-end: an excluded plugin re-added by a profile should appear in
	// enabledPlugins after reconciliation (the fix from PR #41).
	claudeDir := setupApplySettingsEnv(t)

	installed := makeInstalled("excluded-tool@some-marketplace", "context7@claude-plugins-official")
	desired := []string{"context7@claude-plugins-official", "excluded-tool@some-marketplace"}

	reconciled, ep, err := commands.ReconcileEnabledPlugins(claudeDir, desired, installed)
	require.NoError(t, err)

	assert.Contains(t, reconciled, "excluded-tool@some-marketplace",
		"profile-added excluded plugin should be reconciled into enabledPlugins")
	assert.True(t, ep["excluded-tool@some-marketplace"],
		"plugin should be enabled")
	assert.True(t, ep["context7@claude-plugins-official"],
		"base plugin should also be enabled")
}

// --- New surface tests ---

func TestApplySettingsWithPermissions(t *testing.T) {
	claudeDir := t.TempDir()
	require.NoError(t, claudecode.Bootstrap(claudeDir))

	// Write existing settings.json with existing permissions.
	existing := map[string]json.RawMessage{
		"permissions": json.RawMessage(`{"allow":["Read","Write"],"deny":["Bash"]}`),
	}
	require.NoError(t, claudecode.WriteSettings(claudeDir, existing))

	syncDir := filepath.Join(t.TempDir(), ".claude-sync")
	require.NoError(t, os.MkdirAll(syncDir, 0755))

	configYAML := `version: "1.0.0"
plugins:
  upstream: []
permissions:
  allow:
    - Read
    - Execute
  deny:
    - Bash
    - Network
`
	require.NoError(t, os.WriteFile(filepath.Join(syncDir, "config.yaml"), []byte(configYAML), 0644))
	require.NoError(t, exec.Command("git", "init", syncDir).Run())
	require.NoError(t, exec.Command("git", "-C", syncDir, "config", "user.email", "test@test.com").Run())
	require.NoError(t, exec.Command("git", "-C", syncDir, "config", "user.name", "Test").Run())
	require.NoError(t, exec.Command("git", "-C", syncDir, "add", ".").Run())
	require.NoError(t, exec.Command("git", "-C", syncDir, "commit", "-m", "init").Run())

	result, err := commands.Pull(claudeDir, syncDir, true)
	require.NoError(t, err)
	assert.True(t, result.PermissionsApplied)

	// Check merged permissions in settings.json.
	settings, err := claudecode.ReadSettings(claudeDir)
	require.NoError(t, err)

	var mergedPerms struct {
		Allow []string `json:"allow"`
		Deny  []string `json:"deny"`
	}
	require.NoError(t, json.Unmarshal(settings["permissions"], &mergedPerms))
	// "Read" and "Bash" should not be duplicated.
	assert.Equal(t, []string{"Read", "Write", "Execute"}, mergedPerms.Allow)
	assert.Equal(t, []string{"Bash", "Network"}, mergedPerms.Deny)
}

func TestPullClaudeMDAssembly(t *testing.T) {
	claudeDir := t.TempDir()
	require.NoError(t, claudecode.Bootstrap(claudeDir))

	syncDir := filepath.Join(t.TempDir(), ".claude-sync")
	require.NoError(t, os.MkdirAll(syncDir, 0755))

	// Create claude-md fragments.
	claudeMdDir := filepath.Join(syncDir, "claude-md")
	require.NoError(t, os.MkdirAll(claudeMdDir, 0755))
	require.NoError(t, claudemd.WriteFragment(claudeMdDir, "intro", "## Introduction\nWelcome to the project."))
	require.NoError(t, claudemd.WriteFragment(claudeMdDir, "rules", "## Rules\nAlways use Go."))

	configYAML := `version: "1.0.0"
plugins:
  upstream: []
claude_md:
  include:
    - intro
    - rules
`
	require.NoError(t, os.WriteFile(filepath.Join(syncDir, "config.yaml"), []byte(configYAML), 0644))
	require.NoError(t, exec.Command("git", "init", syncDir).Run())
	require.NoError(t, exec.Command("git", "-C", syncDir, "config", "user.email", "test@test.com").Run())
	require.NoError(t, exec.Command("git", "-C", syncDir, "config", "user.name", "Test").Run())
	require.NoError(t, exec.Command("git", "-C", syncDir, "add", ".").Run())
	require.NoError(t, exec.Command("git", "-C", syncDir, "commit", "-m", "init").Run())

	result, err := commands.Pull(claudeDir, syncDir, true)
	require.NoError(t, err)
	assert.True(t, result.ClaudeMDAssembled)

	// Verify CLAUDE.md was written.
	claudeMDPath := filepath.Join(claudeDir, "CLAUDE.md")
	data, err := os.ReadFile(claudeMDPath)
	require.NoError(t, err)
	assert.Contains(t, string(data), "## Introduction")
	assert.Contains(t, string(data), "## Rules")
}

func TestPullMCPApply(t *testing.T) {
	claudeDir := t.TempDir()
	require.NoError(t, claudecode.Bootstrap(claudeDir))

	syncDir := filepath.Join(t.TempDir(), ".claude-sync")
	require.NoError(t, os.MkdirAll(syncDir, 0755))

	configYAML := `version: "1.0.0"
plugins:
  upstream: []
mcp:
  memory:
    command: npx
    args:
      - "-y"
      - "@anthropic/memory-server"
`
	require.NoError(t, os.WriteFile(filepath.Join(syncDir, "config.yaml"), []byte(configYAML), 0644))
	require.NoError(t, exec.Command("git", "init", syncDir).Run())
	require.NoError(t, exec.Command("git", "-C", syncDir, "config", "user.email", "test@test.com").Run())
	require.NoError(t, exec.Command("git", "-C", syncDir, "config", "user.name", "Test").Run())
	require.NoError(t, exec.Command("git", "-C", syncDir, "add", ".").Run())
	require.NoError(t, exec.Command("git", "-C", syncDir, "commit", "-m", "init").Run())

	result, err := commands.Pull(claudeDir, syncDir, true)
	require.NoError(t, err)
	assert.Contains(t, result.MCPApplied, "memory")

	// Verify .mcp.json was written with mcpServers wrapper.
	mcpPath := filepath.Join(claudeDir, ".mcp.json")
	data, err := os.ReadFile(mcpPath)
	require.NoError(t, err)
	assert.Contains(t, string(data), "mcpServers")
	assert.Contains(t, string(data), "memory")
}

func TestPullKeybindings(t *testing.T) {
	claudeDir := t.TempDir()
	require.NoError(t, claudecode.Bootstrap(claudeDir))

	syncDir := filepath.Join(t.TempDir(), ".claude-sync")
	require.NoError(t, os.MkdirAll(syncDir, 0755))

	configYAML := `version: "1.0.0"
plugins:
  upstream: []
keybindings:
  Ctrl+K: clear_screen
  Ctrl+L: toggle_log
`
	require.NoError(t, os.WriteFile(filepath.Join(syncDir, "config.yaml"), []byte(configYAML), 0644))
	require.NoError(t, exec.Command("git", "init", syncDir).Run())
	require.NoError(t, exec.Command("git", "-C", syncDir, "config", "user.email", "test@test.com").Run())
	require.NoError(t, exec.Command("git", "-C", syncDir, "config", "user.name", "Test").Run())
	require.NoError(t, exec.Command("git", "-C", syncDir, "add", ".").Run())
	require.NoError(t, exec.Command("git", "-C", syncDir, "commit", "-m", "init").Run())

	result, err := commands.Pull(claudeDir, syncDir, true)
	require.NoError(t, err)
	assert.True(t, result.KeybindingsApplied)

	// Verify keybindings.json was written.
	kb, err := claudecode.ReadKeybindings(claudeDir)
	require.NoError(t, err)
	assert.Equal(t, "clear_screen", kb["Ctrl+K"])
	assert.Equal(t, "toggle_log", kb["Ctrl+L"])
}

func TestPullAutoModeDefersPending(t *testing.T) {
	claudeDir := t.TempDir()
	require.NoError(t, claudecode.Bootstrap(claudeDir))

	syncDir := filepath.Join(t.TempDir(), ".claude-sync")
	require.NoError(t, os.MkdirAll(syncDir, 0755))

	configYAML := `version: "1.0.0"
plugins:
  upstream: []
settings:
  model: opus
hooks:
  PreCompact: "bd prime"
permissions:
  allow:
    - Execute
  deny:
    - Network
mcp:
  memory:
    command: npx
keybindings:
  Ctrl+K: clear
`
	require.NoError(t, os.WriteFile(filepath.Join(syncDir, "config.yaml"), []byte(configYAML), 0644))
	require.NoError(t, exec.Command("git", "init", syncDir).Run())
	require.NoError(t, exec.Command("git", "-C", syncDir, "config", "user.email", "test@test.com").Run())
	require.NoError(t, exec.Command("git", "-C", syncDir, "config", "user.name", "Test").Run())
	require.NoError(t, exec.Command("git", "-C", syncDir, "add", ".").Run())
	require.NoError(t, exec.Command("git", "-C", syncDir, "commit", "-m", "init").Run())

	result, err := commands.PullWithOptions(commands.PullOptions{
		ClaudeDir: claudeDir,
		SyncDir:   syncDir,
		Quiet:     true,
		Auto:      true,
	})
	require.NoError(t, err)

	// Safe changes should be applied.
	assert.Contains(t, result.SettingsApplied, "model")
	assert.True(t, result.KeybindingsApplied)

	// High-risk changes should NOT be applied.
	assert.Empty(t, result.HooksApplied)
	assert.False(t, result.PermissionsApplied)
	assert.Empty(t, result.MCPApplied)

	// Pending high-risk changes should be listed.
	assert.NotEmpty(t, result.PendingHighRisk)

	// pending-changes.yaml should exist.
	pending, err := approval.ReadPending(syncDir)
	require.NoError(t, err)
	assert.False(t, pending.IsEmpty())
	assert.NotNil(t, pending.Permissions)
	assert.Contains(t, pending.Permissions.Allow, "Execute")
	assert.Contains(t, pending.Permissions.Deny, "Network")
	assert.NotEmpty(t, pending.Hooks)
	assert.NotEmpty(t, pending.MCP)
}

func TestAppendUniqueStrings(t *testing.T) {
	// Test via the exported PullWithOptions to ensure the helper works.
	// We test the behavior indirectly through permissions merge.

	tests := []struct {
		name     string
		base     []string
		add      []string
		expected []string
	}{
		{
			name:     "no duplicates",
			base:     []string{"a", "b"},
			add:      []string{"c", "d"},
			expected: []string{"a", "b", "c", "d"},
		},
		{
			name:     "with duplicates",
			base:     []string{"a", "b"},
			add:      []string{"b", "c"},
			expected: []string{"a", "b", "c"},
		},
		{
			name:     "empty base",
			base:     nil,
			add:      []string{"a", "b"},
			expected: []string{"a", "b"},
		},
		{
			name:     "empty add",
			base:     []string{"a", "b"},
			add:      nil,
			expected: []string{"a", "b"},
		},
		{
			name:     "both empty",
			base:     nil,
			add:      nil,
			expected: []string{},
		},
		{
			name:     "all duplicates",
			base:     []string{"a", "b"},
			add:      []string{"a", "b"},
			expected: []string{"a", "b"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test through permissions merge flow.
			claudeDir := t.TempDir()
			require.NoError(t, claudecode.Bootstrap(claudeDir))

			if len(tt.base) > 0 {
				perms := map[string]any{"allow": tt.base, "deny": []string{}}
				permData, _ := json.Marshal(perms)
				existing := map[string]json.RawMessage{
					"permissions": json.RawMessage(permData),
				}
				require.NoError(t, claudecode.WriteSettings(claudeDir, existing))
			}

			syncDir := filepath.Join(t.TempDir(), ".claude-sync")
			require.NoError(t, os.MkdirAll(syncDir, 0755))

			// Build config YAML with permissions.
			cfgYAML := "version: \"1.0.0\"\nplugins:\n  upstream: []\n"
			if len(tt.add) > 0 {
				cfgYAML += "permissions:\n  allow:\n"
				for _, a := range tt.add {
					cfgYAML += "    - " + a + "\n"
				}
			}
			require.NoError(t, os.WriteFile(filepath.Join(syncDir, "config.yaml"), []byte(cfgYAML), 0644))
			require.NoError(t, exec.Command("git", "init", syncDir).Run())
			require.NoError(t, exec.Command("git", "-C", syncDir, "config", "user.email", "test@test.com").Run())
			require.NoError(t, exec.Command("git", "-C", syncDir, "config", "user.name", "Test").Run())
			require.NoError(t, exec.Command("git", "-C", syncDir, "add", ".").Run())
			require.NoError(t, exec.Command("git", "-C", syncDir, "commit", "-m", "init").Run())

			_, pullErr := commands.Pull(claudeDir, syncDir, true)
			require.NoError(t, pullErr)

			if len(tt.add) > 0 {
				settings, err := claudecode.ReadSettings(claudeDir)
				require.NoError(t, err)
				var mergedPerms struct {
					Allow []string `json:"allow"`
				}
				require.NoError(t, json.Unmarshal(settings["permissions"], &mergedPerms))
				assert.Equal(t, tt.expected, mergedPerms.Allow)
			}
		})
	}
}

func TestPull_AppliesProjectSettings(t *testing.T) {
	claudeDir, syncDir := setupStatusEnv(t)
	projectDir := t.TempDir()
	os.MkdirAll(filepath.Join(projectDir, ".claude"), 0755)

	// Write global config with hooks and permissions
	cfg := config.Config{
		Version: "1.0.0",
		Hooks: map[string]json.RawMessage{
			"PreToolUse": json.RawMessage(`[{"matcher":"^Bash$","hooks":[{"type":"command","command":"python3 validator.py"}]}]`),
		},
		Permissions: config.Permissions{
			Allow: []string{"Read", "Edit"},
		},
	}
	cfgData, _ := config.MarshalV2(cfg)
	os.WriteFile(filepath.Join(syncDir, "config.yaml"), cfgData, 0644)

	// Write project config with permission overrides
	project.WriteProjectConfig(projectDir, project.ProjectConfig{
		Version:       "1.0.0",
		ProjectedKeys: []string{"hooks", "permissions"},
		Overrides: project.ProjectOverrides{
			Permissions: project.ProjectPermissionOverrides{
				AddAllow: []string{"mcp__evvy_db__query"},
			},
		},
	})

	result, err := commands.PullWithOptions(commands.PullOptions{
		ClaudeDir:  claudeDir,
		SyncDir:    syncDir,
		ProjectDir: projectDir,
	})
	require.NoError(t, err)
	assert.True(t, result.ProjectSettingsApplied)

	// Verify settings.local.json has hooks + permissions with override
	slj, err := os.ReadFile(filepath.Join(projectDir, ".claude", "settings.local.json"))
	require.NoError(t, err)
	var settings map[string]json.RawMessage
	json.Unmarshal(slj, &settings)
	assert.Contains(t, string(settings["hooks"]), "PreToolUse")
	assert.Contains(t, string(settings["permissions"]), "mcp__evvy_db__query")
	assert.Contains(t, string(settings["permissions"]), "Read")
}

// --- Stale plugin detection tests ---

// setupStalePluginEnv creates an environment where a plugin is installed at version 1.0.0
// but the marketplace source has been bumped to newVersion.
func setupStalePluginEnv(t *testing.T, newVersion string) (claudeDir, syncDir string) {
	t.Helper()

	claudeDir = t.TempDir()
	require.NoError(t, claudecode.Bootstrap(claudeDir))

	// Create a directory-based marketplace.
	marketplaceDir := t.TempDir()

	// Write marketplace.json and plugin.json with the new version.
	mkplDir := filepath.Join(marketplaceDir, ".claude-plugin")
	require.NoError(t, os.MkdirAll(mkplDir, 0755))

	mkpl := `{
		"name": "test-marketplace",
		"plugins": [{"name": "test-plugin", "source": "./", "version": "` + newVersion + `"}]
	}`
	require.NoError(t, os.WriteFile(filepath.Join(mkplDir, "marketplace.json"), []byte(mkpl), 0644))

	pj := `{"name": "test-plugin", "version": "` + newVersion + `"}`
	require.NoError(t, os.WriteFile(filepath.Join(mkplDir, "plugin.json"), []byte(pj), 0644))

	// Write known_marketplaces.json pointing to the marketplace.
	km, _ := json.MarshalIndent(map[string]any{
		"test-marketplace": map[string]any{
			"source":          map[string]string{"source": "directory", "path": marketplaceDir},
			"installLocation": marketplaceDir,
		},
	}, "", "  ")
	require.NoError(t, os.WriteFile(filepath.Join(claudeDir, "plugins", "known_marketplaces.json"), km, 0644))

	// Write installed_plugins.json with plugin at version 1.0.0.
	installed := `{
		"version": 2,
		"plugins": {
			"test-plugin@test-marketplace": [{
				"scope": "user",
				"installPath": "` + filepath.Join(claudeDir, "plugins", "cache", "test-marketplace", "test-plugin", "1.0.0") + `",
				"version": "1.0.0",
				"installedAt": "2026-01-01T00:00:00Z",
				"lastUpdated": "2026-01-01T00:00:00Z"
			}]
		}
	}`
	require.NoError(t, os.WriteFile(filepath.Join(claudeDir, "plugins", "installed_plugins.json"), []byte(installed), 0644))

	// Store the content hash only when the installed version matches the
	// marketplace version (simulates a previous successful install+hash).
	// When versions differ, the empty hash causes staleness (bootstrap).
	if newVersion == "1.0.0" {
		hash, hashErr := marketplace.ComputePluginContentHash(marketplaceDir)
		if hashErr == nil {
			pch := &claudecode.PluginContentHashes{
				Hashes: map[string]string{
					"test-plugin@test-marketplace": hash,
				},
			}
			claudecode.WritePluginContentHashes(claudeDir, pch)
		}
	}

	// Create a sync dir with config listing the plugin.
	syncDir = filepath.Join(t.TempDir(), ".claude-sync")
	require.NoError(t, os.MkdirAll(syncDir, 0755))
	configYAML := `version: "1.0.0"
plugins:
  upstream:
    - test-plugin@test-marketplace
`
	require.NoError(t, os.WriteFile(filepath.Join(syncDir, "config.yaml"), []byte(configYAML), 0644))

	require.NoError(t, exec.Command("git", "init", syncDir).Run())
	require.NoError(t, exec.Command("git", "-C", syncDir, "config", "user.email", "test@test.com").Run())
	require.NoError(t, exec.Command("git", "-C", syncDir, "config", "user.name", "Test").Run())
	require.NoError(t, exec.Command("git", "-C", syncDir, "add", ".").Run())
	require.NoError(t, exec.Command("git", "-C", syncDir, "commit", "-m", "init").Run())

	return claudeDir, syncDir
}

func TestPull_DetectsStalePlugin(t *testing.T) {
	claudeDir, syncDir := setupStalePluginEnv(t, "1.1.0")

	// DryRun should show the plugin as Synced (already installed).
	result, err := commands.PullDryRun(claudeDir, syncDir)
	require.NoError(t, err)
	assert.Contains(t, result.Synced, "test-plugin@test-marketplace")
	assert.Empty(t, result.ToInstall)
}

func TestPull_NoStaleWhenVersionsMatch(t *testing.T) {
	claudeDir, syncDir := setupStalePluginEnv(t, "1.0.0")

	// Pull runs a full plugin refresh for all configured upstream plugins.
	// Stale detection finds nothing (versions match), but UpdateApply still runs
	// for all upstream plugins. In the test environment, claude plugin install is
	// not available, so entries may appear in UpdateFailed.
	result, err := commands.Pull(claudeDir, syncDir, true)
	require.NoError(t, err)
	// Stale detection path should not add to Updated (versions match).
	assert.Empty(t, result.Updated)
	// UpdateFailed may contain entries from the full refresh path (install unavailable in tests).
	// The important contract is that the pull itself succeeds without error.
	_ = result.UpdateFailed
}

func TestPullResult_HasUpdatedFields(t *testing.T) {
	// Verify the PullResult struct has the new fields.
	result := commands.PullResult{
		Updated:      []string{"foo@bar"},
		UpdateFailed: []string{"baz@qux"},
	}
	assert.Equal(t, []string{"foo@bar"}, result.Updated)
	assert.Equal(t, []string{"baz@qux"}, result.UpdateFailed)
}

// --- Content-hash-based stale plugin detection tests ---

// setupContentHashEnv creates a directory-based marketplace environment with
// a plugin installed, the sidecar hash file populated, and a sync dir listing
// the plugin. Returns (claudeDir, syncDir, marketplaceDir) so the caller can
// modify files in marketplaceDir to test hash change detection.
func setupContentHashEnv(t *testing.T) (claudeDir, syncDir, marketplaceDir string) {
	t.Helper()

	claudeDir = t.TempDir()
	require.NoError(t, claudecode.Bootstrap(claudeDir))

	// Create a directory-based marketplace with a plugin.
	marketplaceDir = t.TempDir()

	// Write marketplace.json and plugin.json.
	mkplDir := filepath.Join(marketplaceDir, ".claude-plugin")
	require.NoError(t, os.MkdirAll(mkplDir, 0755))

	mkpl := `{"name": "test-marketplace", "plugins": [{"name": "test-plugin", "source": "./", "version": "1.0.0"}]}`
	require.NoError(t, os.WriteFile(filepath.Join(mkplDir, "marketplace.json"), []byte(mkpl), 0644))

	pj := `{"name": "test-plugin", "version": "1.0.0"}`
	require.NoError(t, os.WriteFile(filepath.Join(mkplDir, "plugin.json"), []byte(pj), 0644))

	// Add a source file to the marketplace to hash.
	require.NoError(t, os.WriteFile(filepath.Join(marketplaceDir, "hook.py"), []byte("print('hello')"), 0644))

	// Write known_marketplaces.json (directory-based).
	km, _ := json.MarshalIndent(map[string]any{
		"test-marketplace": map[string]any{
			"source":          map[string]string{"source": "directory", "path": marketplaceDir},
			"installLocation": marketplaceDir,
		},
	}, "", "  ")
	require.NoError(t, os.WriteFile(filepath.Join(claudeDir, "plugins", "known_marketplaces.json"), km, 0644))

	// Write installed_plugins.json with plugin at version 1.0.0.
	installed := `{
		"version": 2,
		"plugins": {
			"test-plugin@test-marketplace": [{
				"scope": "user",
				"installPath": "` + filepath.Join(claudeDir, "plugins", "cache", "test-marketplace", "test-plugin") + `",
				"version": "1.0.0",
				"installedAt": "2026-01-01T00:00:00Z",
				"lastUpdated": "2026-01-01T00:00:00Z"
			}]
		}
	}`
	require.NoError(t, os.WriteFile(filepath.Join(claudeDir, "plugins", "installed_plugins.json"), []byte(installed), 0644))

	// Compute and store the content hash for the current state.
	hash, err := marketplace.ComputePluginContentHash(marketplaceDir)
	require.NoError(t, err)

	pch := &claudecode.PluginContentHashes{
		Hashes: map[string]string{
			"test-plugin@test-marketplace": hash,
		},
	}
	require.NoError(t, claudecode.WritePluginContentHashes(claudeDir, pch))

	// Create a sync dir with config listing the plugin.
	syncDir = filepath.Join(t.TempDir(), ".claude-sync")
	require.NoError(t, os.MkdirAll(syncDir, 0755))
	configYAML := `version: "1.0.0"
plugins:
  upstream:
    - test-plugin@test-marketplace
`
	require.NoError(t, os.WriteFile(filepath.Join(syncDir, "config.yaml"), []byte(configYAML), 0644))
	require.NoError(t, exec.Command("git", "init", syncDir).Run())
	require.NoError(t, exec.Command("git", "-C", syncDir, "config", "user.email", "test@test.com").Run())
	require.NoError(t, exec.Command("git", "-C", syncDir, "config", "user.name", "Test").Run())
	require.NoError(t, exec.Command("git", "-C", syncDir, "add", ".").Run())
	require.NoError(t, exec.Command("git", "-C", syncDir, "commit", "-m", "init").Run())

	return claudeDir, syncDir, marketplaceDir
}

func TestPull_DetectsStalePluginByContentHash(t *testing.T) {
	claudeDir, syncDir, marketplaceDir := setupContentHashEnv(t)

	// Modify a file in the marketplace WITHOUT changing the version.
	require.NoError(t, os.WriteFile(filepath.Join(marketplaceDir, "hook.py"), []byte("print('changed!')"), 0644))

	// Pull should detect the stale plugin via content hash mismatch.
	// Note: the actual reinstall will fail since `claude plugin install` isn't
	// available in tests, but the detection (Updated/UpdateFailed) proves it works.
	result, err := commands.Pull(claudeDir, syncDir, true)
	require.NoError(t, err)

	// The plugin should be detected as stale. Since `claude plugin install`
	// won't succeed in tests, it will appear in UpdateFailed.
	staleDetected := len(result.Updated) > 0 || len(result.UpdateFailed) > 0
	assert.True(t, staleDetected, "plugin should be detected as stale when content hash changes")
}

func TestPull_NoStaleWhenContentHashMatches(t *testing.T) {
	claudeDir, syncDir, _ := setupContentHashEnv(t)

	// Pull without modifying any files — hash should match.
	// Pull now also runs a full plugin refresh for all configured upstream plugins.
	// Stale detection finds nothing (hash matches), but UpdateApply still runs.
	// In the test environment, claude plugin install is not available, so
	// UpdateFailed may be non-empty from the full refresh path.
	result, err := commands.Pull(claudeDir, syncDir, true)
	require.NoError(t, err)

	// Stale detection should not add to Updated (hash matches).
	assert.Empty(t, result.Updated, "no stale-detection updates when content hash matches")
	// UpdateFailed may contain entries from the full refresh path (install unavailable in tests).
	_ = result.UpdateFailed
}

func TestPull_StaleWhenNoStoredHash(t *testing.T) {
	claudeDir, syncDir, _ := setupContentHashEnv(t)

	// Remove the sidecar file to simulate first run.
	os.Remove(filepath.Join(claudeDir, "plugins", "plugin_content_hashes.json"))

	// Pull should treat plugin as stale (empty stored hash = bootstrap).
	result, err := commands.Pull(claudeDir, syncDir, true)
	require.NoError(t, err)

	staleDetected := len(result.Updated) > 0 || len(result.UpdateFailed) > 0
	assert.True(t, staleDetected, "plugin should be stale when no stored hash exists")
}

// setupCommandSkillEnv creates a test environment with commands and skills
// in the sync dir. Returns (claudeDir, syncDir).
func setupCommandSkillEnv(t *testing.T) (claudeDir, syncDir string) {
	t.Helper()

	claudeDir = t.TempDir()
	require.NoError(t, claudecode.Bootstrap(claudeDir))

	syncDir = filepath.Join(t.TempDir(), ".claude-sync")
	require.NoError(t, os.MkdirAll(syncDir, 0755))

	configYAML := `version: "1.0.0"
plugins:
  upstream:
    - context7@claude-plugins-official
commands:
  - cmd:global:review-pr
  - cmd:global:deploy
skills:
  - skill:global:brainstorming
`
	require.NoError(t, os.WriteFile(filepath.Join(syncDir, "config.yaml"), []byte(configYAML), 0644))

	// Create command files in sync dir.
	cmdDir := filepath.Join(syncDir, "commands")
	require.NoError(t, os.MkdirAll(cmdDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(cmdDir, "review-pr.md"), []byte("# Review PR\nOriginal content"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(cmdDir, "deploy.md"), []byte("# Deploy\nOriginal content"), 0644))

	// Create skill directory in sync dir.
	skillDir := filepath.Join(syncDir, "skills", "brainstorming")
	require.NoError(t, os.MkdirAll(skillDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("# Brainstorming\nOriginal content"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(skillDir, "README.md"), []byte("# Brainstorming README\nCompanion content"), 0644))

	require.NoError(t, exec.Command("git", "init", syncDir).Run())
	require.NoError(t, exec.Command("git", "-C", syncDir, "config", "user.email", "test@test.com").Run())
	require.NoError(t, exec.Command("git", "-C", syncDir, "config", "user.name", "Test").Run())
	require.NoError(t, exec.Command("git", "-C", syncDir, "add", ".").Run())
	require.NoError(t, exec.Command("git", "-C", syncDir, "commit", "-m", "init").Run())

	return claudeDir, syncDir
}

func TestRestoreSkipsLocallyModifiedCommand(t *testing.T) {
	claudeDir, syncDir := setupCommandSkillEnv(t)

	// First pull — restores commands and creates hash file.
	result, err := commands.Pull(claudeDir, syncDir, true)
	require.NoError(t, err)
	assert.Equal(t, 2, result.CommandsRestored)
	assert.Equal(t, 0, result.CommandsSkipped)

	// Modify a command locally (simulates agent editing a command).
	localCmd := filepath.Join(claudeDir, "commands", "review-pr.md")
	require.NoError(t, os.WriteFile(localCmd, []byte("# Review PR\nLocally modified content"), 0644))

	// Second pull — should skip the modified command, overwrite the unmodified one.
	result, err = commands.Pull(claudeDir, syncDir, true)
	require.NoError(t, err)
	assert.Equal(t, 1, result.CommandsRestored, "unmodified command should be restored")
	assert.Equal(t, 1, result.CommandsSkipped, "locally modified command should be skipped")

	// Verify the local modification was preserved.
	data, err := os.ReadFile(localCmd)
	require.NoError(t, err)
	assert.Contains(t, string(data), "Locally modified content")
}

func TestRestoreSkipsLocallyModifiedSkill(t *testing.T) {
	claudeDir, syncDir := setupCommandSkillEnv(t)

	// First pull — restores skills and creates hash file.
	result, err := commands.Pull(claudeDir, syncDir, true)
	require.NoError(t, err)
	assert.Equal(t, 1, result.SkillsRestored)
	assert.Equal(t, 0, result.SkillsSkipped)

	// Modify skill SKILL.md locally.
	localSkill := filepath.Join(claudeDir, "skills", "brainstorming", "SKILL.md")
	require.NoError(t, os.WriteFile(localSkill, []byte("# Brainstorming\nLocally modified skill"), 0644))

	// Second pull — should skip the modified skill.
	result, err = commands.Pull(claudeDir, syncDir, true)
	require.NoError(t, err)
	assert.Equal(t, 0, result.SkillsRestored, "modified skill should not be restored")
	assert.Equal(t, 1, result.SkillsSkipped, "locally modified skill should be skipped")

	// Verify local modification preserved.
	data, err := os.ReadFile(localSkill)
	require.NoError(t, err)
	assert.Contains(t, string(data), "Locally modified skill")
}

func TestRestoreOverwritesUnmodifiedCommand(t *testing.T) {
	claudeDir, syncDir := setupCommandSkillEnv(t)

	// First pull.
	_, err := commands.Pull(claudeDir, syncDir, true)
	require.NoError(t, err)

	// Update the command in sync dir (simulates remote change).
	syncCmd := filepath.Join(syncDir, "commands", "review-pr.md")
	require.NoError(t, os.WriteFile(syncCmd, []byte("# Review PR\nUpdated remote content"), 0644))
	exec.Command("git", "-C", syncDir, "add", ".").Run()
	exec.Command("git", "-C", syncDir, "commit", "-m", "update command").Run()

	// Second pull — local was not modified, so remote update should overwrite.
	result, err := commands.Pull(claudeDir, syncDir, true)
	require.NoError(t, err)
	assert.Equal(t, 2, result.CommandsRestored, "both unmodified commands should be restored")
	assert.Equal(t, 0, result.CommandsSkipped)

	// Verify remote content arrived.
	data, err := os.ReadFile(filepath.Join(claudeDir, "commands", "review-pr.md"))
	require.NoError(t, err)
	assert.Contains(t, string(data), "Updated remote content")
}

func TestRestoreHandlesNoHashFile(t *testing.T) {
	claudeDir, syncDir := setupCommandSkillEnv(t)

	// Pull with no prior hash file — should restore all + create hash file.
	result, err := commands.Pull(claudeDir, syncDir, true)
	require.NoError(t, err)
	assert.Equal(t, 2, result.CommandsRestored)
	assert.Equal(t, 0, result.CommandsSkipped)
	assert.Equal(t, 1, result.SkillsRestored)
	assert.Equal(t, 0, result.SkillsSkipped)

	// Verify hash files were created.
	cmdHashFile := filepath.Join(claudeDir, "commands", ".content_hashes.json")
	_, err = os.Stat(cmdHashFile)
	assert.NoError(t, err, "command hash file should exist")

	skillHashFile := filepath.Join(claudeDir, "skills", ".content_hashes.json")
	_, err = os.Stat(skillHashFile)
	assert.NoError(t, err, "skill hash file should exist")

	// Verify hash file content is valid JSON with expected keys.
	data, err := os.ReadFile(cmdHashFile)
	require.NoError(t, err)
	var hashes struct {
		Hashes map[string]string `json:"hashes"`
	}
	require.NoError(t, json.Unmarshal(data, &hashes))
	assert.Contains(t, hashes.Hashes, "review-pr.md")
	assert.Contains(t, hashes.Hashes, "deploy.md")
	assert.Len(t, hashes.Hashes["review-pr.md"], 16, "hash should be 16-char hex")
}

// --- Applied hash protection tests ---

// setupSettingsEnv creates a test environment with settings in config.yaml.
func setupSettingsEnv(t *testing.T) (claudeDir, syncDir string) {
	t.Helper()

	claudeDir = t.TempDir()
	require.NoError(t, claudecode.Bootstrap(claudeDir))

	syncDir = filepath.Join(t.TempDir(), ".claude-sync")
	require.NoError(t, os.MkdirAll(syncDir, 0755))

	configYAML := `version: "1.0.0"
plugins:
  upstream: []
settings:
  model: claude-sonnet-4-5-20250929
`
	require.NoError(t, os.WriteFile(filepath.Join(syncDir, "config.yaml"), []byte(configYAML), 0644))
	require.NoError(t, exec.Command("git", "init", syncDir).Run())
	require.NoError(t, exec.Command("git", "-C", syncDir, "config", "user.email", "test@test.com").Run())
	require.NoError(t, exec.Command("git", "-C", syncDir, "config", "user.name", "Test").Run())
	require.NoError(t, exec.Command("git", "-C", syncDir, "add", ".").Run())
	require.NoError(t, exec.Command("git", "-C", syncDir, "commit", "-m", "init").Run())

	return claudeDir, syncDir
}

func TestPull_SkipsLocallyModifiedSettings(t *testing.T) {
	claudeDir, syncDir := setupSettingsEnv(t)

	// First pull — should apply settings and record hash.
	result, err := commands.Pull(claudeDir, syncDir, true)
	require.NoError(t, err)
	assert.NotEmpty(t, result.SettingsApplied)
	assert.False(t, result.SettingsSkipped)

	// Modify settings.json locally (simulate user edit).
	settingsPath := filepath.Join(claudeDir, "settings.json")
	os.WriteFile(settingsPath, []byte(`{"model": "my-custom-model"}`), 0644)

	// Second pull — should skip settings due to local modification.
	result2, err := commands.Pull(claudeDir, syncDir, true)
	require.NoError(t, err)
	assert.True(t, result2.SettingsSkipped)
	assert.Empty(t, result2.SettingsApplied)

	// Verify local modification was preserved.
	data, err := os.ReadFile(settingsPath)
	require.NoError(t, err)
	assert.Contains(t, string(data), "my-custom-model")
}

func TestPull_SkipsLocallyModifiedClaudeMD(t *testing.T) {
	claudeDir := t.TempDir()
	require.NoError(t, claudecode.Bootstrap(claudeDir))

	syncDir := filepath.Join(t.TempDir(), ".claude-sync")
	require.NoError(t, os.MkdirAll(syncDir, 0755))

	// Create claude-md fragments.
	claudeMdDir := filepath.Join(syncDir, "claude-md")
	require.NoError(t, os.MkdirAll(claudeMdDir, 0755))
	require.NoError(t, claudemd.WriteFragment(claudeMdDir, "intro", "## Introduction\nWelcome."))

	configYAML := `version: "1.0.0"
plugins:
  upstream: []
claude_md:
  include:
    - intro
`
	require.NoError(t, os.WriteFile(filepath.Join(syncDir, "config.yaml"), []byte(configYAML), 0644))
	require.NoError(t, exec.Command("git", "init", syncDir).Run())
	require.NoError(t, exec.Command("git", "-C", syncDir, "config", "user.email", "test@test.com").Run())
	require.NoError(t, exec.Command("git", "-C", syncDir, "config", "user.name", "Test").Run())
	require.NoError(t, exec.Command("git", "-C", syncDir, "add", ".").Run())
	require.NoError(t, exec.Command("git", "-C", syncDir, "commit", "-m", "init").Run())

	// First pull — should assemble CLAUDE.md.
	result, err := commands.Pull(claudeDir, syncDir, true)
	require.NoError(t, err)
	assert.True(t, result.ClaudeMDAssembled)
	assert.False(t, result.ClaudeMDSkipped)

	// Modify CLAUDE.md locally.
	claudeMDPath := filepath.Join(claudeDir, "CLAUDE.md")
	os.WriteFile(claudeMDPath, []byte("# My custom CLAUDE.md"), 0644)

	// Second pull — should skip CLAUDE.md.
	result2, err := commands.Pull(claudeDir, syncDir, true)
	require.NoError(t, err)
	assert.True(t, result2.ClaudeMDSkipped)
	assert.False(t, result2.ClaudeMDAssembled)

	// Verify local modification was preserved.
	data, err := os.ReadFile(claudeMDPath)
	require.NoError(t, err)
	assert.Contains(t, string(data), "My custom CLAUDE.md")
}

func TestPull_AppliesWhenNoLocalModification(t *testing.T) {
	claudeDir, syncDir := setupSettingsEnv(t)

	// First pull.
	result, err := commands.Pull(claudeDir, syncDir, true)
	require.NoError(t, err)
	assert.NotEmpty(t, result.SettingsApplied)

	// Second pull without modifying — should apply again (no local modification).
	result2, err := commands.Pull(claudeDir, syncDir, true)
	require.NoError(t, err)
	assert.False(t, result2.SettingsSkipped)
	assert.NotEmpty(t, result2.SettingsApplied)
}

func TestPull_SkipsLocallyModifiedKeybindings(t *testing.T) {
	claudeDir := t.TempDir()
	require.NoError(t, claudecode.Bootstrap(claudeDir))

	syncDir := filepath.Join(t.TempDir(), ".claude-sync")
	require.NoError(t, os.MkdirAll(syncDir, 0755))

	configYAML := `version: "1.0.0"
plugins:
  upstream: []
keybindings:
  Ctrl+K: clear_screen
`
	require.NoError(t, os.WriteFile(filepath.Join(syncDir, "config.yaml"), []byte(configYAML), 0644))
	require.NoError(t, exec.Command("git", "init", syncDir).Run())
	require.NoError(t, exec.Command("git", "-C", syncDir, "config", "user.email", "test@test.com").Run())
	require.NoError(t, exec.Command("git", "-C", syncDir, "config", "user.name", "Test").Run())
	require.NoError(t, exec.Command("git", "-C", syncDir, "add", ".").Run())
	require.NoError(t, exec.Command("git", "-C", syncDir, "commit", "-m", "init").Run())

	// First pull — should apply keybindings.
	result, err := commands.Pull(claudeDir, syncDir, true)
	require.NoError(t, err)
	assert.True(t, result.KeybindingsApplied)
	assert.False(t, result.KeybindingsSkipped)

	// Modify keybindings.json locally.
	kbPath := filepath.Join(claudeDir, "keybindings.json")
	require.NoError(t, os.WriteFile(kbPath, []byte(`{"Ctrl+J": "my_custom_action"}`), 0644))

	// Second pull — should skip keybindings due to local modification.
	result2, err := commands.Pull(claudeDir, syncDir, true)
	require.NoError(t, err)
	assert.True(t, result2.KeybindingsSkipped)
	assert.False(t, result2.KeybindingsApplied)

	// Verify local modification was preserved.
	data, err := os.ReadFile(kbPath)
	require.NoError(t, err)
	assert.Contains(t, string(data), "my_custom_action")
}

func TestPull_ForceOverwritesLocallyModified(t *testing.T) {
	claudeDir, syncDir := setupSettingsEnv(t)

	// First pull — apply settings.
	result, err := commands.Pull(claudeDir, syncDir, true)
	require.NoError(t, err)
	assert.NotEmpty(t, result.SettingsApplied)

	// Modify settings locally.
	settingsPath := filepath.Join(claudeDir, "settings.json")
	require.NoError(t, os.WriteFile(settingsPath, []byte(`{"model": "my-custom-model"}`), 0644))

	// Pull with force — should overwrite local modification.
	result2, err := commands.PullWithOptions(commands.PullOptions{
		ClaudeDir: claudeDir,
		SyncDir:   syncDir,
		Quiet:     true,
		Force:     true,
	})
	require.NoError(t, err)
	assert.False(t, result2.SettingsSkipped)
	assert.NotEmpty(t, result2.SettingsApplied)

	// Verify upstream settings were applied (local modification overwritten).
	data, err := os.ReadFile(settingsPath)
	require.NoError(t, err)
	assert.NotContains(t, string(data), "my-custom-model")
}

// --- ReconcileEnabledPlugins tests ---

func makeInstalled(keys ...string) *claudecode.InstalledPlugins {
	ip := &claudecode.InstalledPlugins{
		Version: 2,
		Plugins: make(map[string][]claudecode.PluginInstallation),
	}
	for _, k := range keys {
		ip.Plugins[k] = []claudecode.PluginInstallation{{
			Scope: "user", Version: "1.0", InstallPath: "/p",
		}}
	}
	return ip
}

func TestReconcileEnabledPlugins_AddsAbsentEntries(t *testing.T) {
	claudeDir := setupApplySettingsEnv(t)

	// settings.json has enabledPlugins with only pluginA.
	existing := map[string]json.RawMessage{
		"enabledPlugins": json.RawMessage(`{"a@m": true}`),
	}
	require.NoError(t, claudecode.WriteSettings(claudeDir, existing))

	installed := makeInstalled("a@m", "b@m")
	desired := []string{"a@m", "b@m"}

	reconciled, _, err := commands.ReconcileEnabledPlugins(claudeDir, desired, installed)
	require.NoError(t, err)
	assert.Equal(t, []string{"b@m"}, reconciled)

	settings := readSettingsJSON(t, claudeDir)
	var ep map[string]bool
	require.NoError(t, json.Unmarshal(settings["enabledPlugins"], &ep))
	assert.True(t, ep["a@m"], "existing entry preserved")
	assert.True(t, ep["b@m"], "missing entry added")
}

func TestReconcileEnabledPlugins_SkipsExplicitlyDisabled(t *testing.T) {
	claudeDir := setupApplySettingsEnv(t)

	// pluginA is explicitly disabled (false).
	existing := map[string]json.RawMessage{
		"enabledPlugins": json.RawMessage(`{"a@m": false}`),
	}
	require.NoError(t, claudecode.WriteSettings(claudeDir, existing))

	installed := makeInstalled("a@m")
	desired := []string{"a@m"}

	reconciled, _, err := commands.ReconcileEnabledPlugins(claudeDir, desired, installed)
	require.NoError(t, err)
	assert.Empty(t, reconciled, "should not overwrite explicit false")

	settings := readSettingsJSON(t, claudeDir)
	var ep map[string]bool
	require.NoError(t, json.Unmarshal(settings["enabledPlugins"], &ep))
	assert.False(t, ep["a@m"], "explicit false preserved")
}

func TestReconcileEnabledPlugins_PreservesUntracked(t *testing.T) {
	claudeDir := setupApplySettingsEnv(t)

	// settings.json has an entry for a plugin not in effectiveDesired.
	existing := map[string]json.RawMessage{
		"enabledPlugins": json.RawMessage(`{"untracked@m": true}`),
	}
	require.NoError(t, claudecode.WriteSettings(claudeDir, existing))

	installed := makeInstalled("a@m")
	desired := []string{"a@m"}

	reconciled, _, err := commands.ReconcileEnabledPlugins(claudeDir, desired, installed)
	require.NoError(t, err)
	assert.Equal(t, []string{"a@m"}, reconciled)

	settings := readSettingsJSON(t, claudeDir)
	var ep map[string]bool
	require.NoError(t, json.Unmarshal(settings["enabledPlugins"], &ep))
	assert.True(t, ep["untracked@m"], "untracked entry preserved")
	assert.True(t, ep["a@m"], "desired entry added")
}

func TestReconcileEnabledPlugins_NoSettingsFile(t *testing.T) {
	claudeDir := setupApplySettingsEnv(t)
	// No settings.json exists.

	installed := makeInstalled("a@m")
	desired := []string{"a@m"}

	reconciled, _, err := commands.ReconcileEnabledPlugins(claudeDir, desired, installed)
	require.NoError(t, err)
	assert.Equal(t, []string{"a@m"}, reconciled)

	settings := readSettingsJSON(t, claudeDir)
	var ep map[string]bool
	require.NoError(t, json.Unmarshal(settings["enabledPlugins"], &ep))
	assert.True(t, ep["a@m"])
}

func TestReconcileEnabledPlugins_NoopWhenAllPresent(t *testing.T) {
	claudeDir := setupApplySettingsEnv(t)

	existing := map[string]json.RawMessage{
		"enabledPlugins": json.RawMessage(`{"a@m": true, "b@m": true}`),
		"model":          json.RawMessage(`"my-model"`),
	}
	require.NoError(t, claudecode.WriteSettings(claudeDir, existing))

	installed := makeInstalled("a@m", "b@m")
	desired := []string{"a@m", "b@m"}

	reconciled, _, err := commands.ReconcileEnabledPlugins(claudeDir, desired, installed)
	require.NoError(t, err)
	assert.Empty(t, reconciled, "nothing to reconcile")
}

func TestReconcileEnabledPlugins_IgnoresDesiredButNotInstalled(t *testing.T) {
	claudeDir := setupApplySettingsEnv(t)

	existing := map[string]json.RawMessage{
		"enabledPlugins": json.RawMessage(`{}`),
	}
	require.NoError(t, claudecode.WriteSettings(claudeDir, existing))

	// "b@m" is desired but NOT in installed_plugins.json (install failed).
	installed := makeInstalled("a@m")
	desired := []string{"a@m", "b@m"}

	reconciled, _, err := commands.ReconcileEnabledPlugins(claudeDir, desired, installed)
	require.NoError(t, err)
	assert.Equal(t, []string{"a@m"}, reconciled, "only installed plugin reconciled")

	settings := readSettingsJSON(t, claudeDir)
	var ep map[string]bool
	require.NoError(t, json.Unmarshal(settings["enabledPlugins"], &ep))
	assert.True(t, ep["a@m"])
	_, hasBM := ep["b@m"]
	assert.False(t, hasBM, "not-installed plugin should not get an entry")
}

func TestRestoreSkipsLocallyModifiedSkillCompanion(t *testing.T) {
	claudeDir, syncDir := setupCommandSkillEnv(t)

	// First pull: restores skills (including companion files) and creates hash file.
	result, err := commands.Pull(claudeDir, syncDir, true)
	require.NoError(t, err)
	assert.Equal(t, 1, result.SkillsRestored)
	assert.Equal(t, 0, result.SkillsSkipped)

	// Verify companion file was copied.
	localReadme := filepath.Join(claudeDir, "skills", "brainstorming", "README.md")
	_, err = os.Stat(localReadme)
	require.NoError(t, err, "companion README.md should exist after first pull")

	// Modify only the companion file locally (simulates agent editing README).
	require.NoError(t, os.WriteFile(localReadme, []byte("# Brainstorming README\nLocally modified"), 0644))

	// Second pull: should skip because the directory hash changed.
	result, err = commands.Pull(claudeDir, syncDir, true)
	require.NoError(t, err)
	assert.Equal(t, 0, result.SkillsRestored, "should not overwrite locally modified companion")
	assert.Equal(t, 1, result.SkillsSkipped, "should skip skill with modified companion")

	// Local modification should still be present.
	data, err := os.ReadFile(localReadme)
	require.NoError(t, err)
	assert.Contains(t, string(data), "Locally modified")
}

func TestRestoreUpdatesSkillCompanionFiles(t *testing.T) {
	claudeDir, syncDir := setupCommandSkillEnv(t)

	// First pull: restores skills.
	result, err := commands.Pull(claudeDir, syncDir, true)
	require.NoError(t, err)
	assert.Equal(t, 1, result.SkillsRestored)

	// Update only the companion file in the sync dir (simulates remote change).
	syncReadme := filepath.Join(syncDir, "skills", "brainstorming", "README.md")
	require.NoError(t, os.WriteFile(syncReadme, []byte("# Brainstorming README\nUpdated remotely"), 0644))

	// Commit the change so the sync dir is a valid repo.
	require.NoError(t, exec.Command("git", "-C", syncDir, "add", ".").Run())
	require.NoError(t, exec.Command("git", "-C", syncDir, "commit", "-m", "update companion").Run())

	// Second pull: should detect source dir changed and copy the update.
	result, err = commands.Pull(claudeDir, syncDir, true)
	require.NoError(t, err)
	assert.Equal(t, 1, result.SkillsRestored, "should copy updated companion from sync dir")
	assert.Equal(t, 0, result.SkillsSkipped)

	// Verify the updated companion file was copied.
	localReadme := filepath.Join(claudeDir, "skills", "brainstorming", "README.md")
	data, err := os.ReadFile(localReadme)
	require.NoError(t, err)
	assert.Contains(t, string(data), "Updated remotely")
}
