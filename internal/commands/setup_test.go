package commands

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSetupAutoSyncHooks_EmptySettings(t *testing.T) {
	claudeDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(claudeDir, "settings.json"), []byte("{}"), 0644))

	err := SetupAutoSyncHooks(claudeDir)
	require.NoError(t, err)

	data, err := os.ReadFile(filepath.Join(claudeDir, "settings.json"))
	require.NoError(t, err)

	var settings map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(data, &settings))

	var hooks map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(settings["hooks"], &hooks))

	assert.Contains(t, string(hooks["PostToolUse"]), "claude-sync auto-commit --if-changed")
	assert.Contains(t, string(hooks["SessionEnd"]), "claude-sync push --auto --quiet")
	assert.Contains(t, string(hooks["SessionStart"]), "claude-sync pull --auto")
}

func TestSetupAutoSyncHooks_PreservesExisting(t *testing.T) {
	claudeDir := t.TempDir()

	existingSettings := map[string]any{
		"model": "opus",
		"hooks": map[string]any{
			"PreToolUse": []any{
				map[string]any{
					"matcher": "Bash",
					"hooks": []any{
						map[string]any{"type": "command", "command": "echo pre-tool"},
					},
				},
			},
		},
	}
	data, err := json.MarshalIndent(existingSettings, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(claudeDir, "settings.json"), data, 0644))

	err = SetupAutoSyncHooks(claudeDir)
	require.NoError(t, err)

	data, err = os.ReadFile(filepath.Join(claudeDir, "settings.json"))
	require.NoError(t, err)

	var settings map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(data, &settings))

	// Model setting should be preserved.
	assert.Contains(t, string(settings["model"]), "opus")

	var hooks map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(settings["hooks"], &hooks))

	// Existing hook should still be present.
	assert.Contains(t, string(hooks["PreToolUse"]), "echo pre-tool")

	// New hooks should be present.
	assert.Contains(t, string(hooks["PostToolUse"]), "claude-sync auto-commit --if-changed")
	assert.Contains(t, string(hooks["SessionEnd"]), "claude-sync push --auto --quiet")
	assert.Contains(t, string(hooks["SessionStart"]), "claude-sync pull --auto")
}

func TestSetupAutoSyncHooks_Idempotent(t *testing.T) {
	claudeDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(claudeDir, "settings.json"), []byte("{}"), 0644))

	// Call twice.
	require.NoError(t, SetupAutoSyncHooks(claudeDir))
	require.NoError(t, SetupAutoSyncHooks(claudeDir))

	data, err := os.ReadFile(filepath.Join(claudeDir, "settings.json"))
	require.NoError(t, err)

	var settings map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(data, &settings))

	var hooks map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(settings["hooks"], &hooks))

	// Each event should only have one hook rule entry.
	for _, eventName := range []string{"PostToolUse", "SessionEnd", "SessionStart"} {
		var rules []json.RawMessage
		require.NoError(t, json.Unmarshal(hooks[eventName], &rules), "event: %s", eventName)
		assert.Len(t, rules, 1, "event %s should have exactly 1 rule after two calls", eventName)
	}
}

func TestSetupAutoSyncHooks_NoSettingsFile(t *testing.T) {
	claudeDir := t.TempDir()
	// No settings.json exists.

	err := SetupAutoSyncHooks(claudeDir)
	require.NoError(t, err)

	data, err := os.ReadFile(filepath.Join(claudeDir, "settings.json"))
	require.NoError(t, err)

	var settings map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(data, &settings))

	var hooks map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(settings["hooks"], &hooks))

	assert.Contains(t, string(hooks["PostToolUse"]), "claude-sync auto-commit --if-changed")
}

func TestSetupAutoSyncHooksDescription(t *testing.T) {
	desc := SetupAutoSyncHooksDescription()
	assert.Contains(t, desc, "PostToolUse")
	assert.Contains(t, desc, "SessionEnd")
	assert.Contains(t, desc, "SessionStart")
	assert.Contains(t, desc, "claude-sync")
}
