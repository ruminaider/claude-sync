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
	"github.com/ruminaider/claude-sync/internal/plugins"
	"github.com/ruminaider/claude-sync/internal/profiles"
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
