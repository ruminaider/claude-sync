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
	// SessionEnd and SessionStart are now handled by the plugin, not settings.json.
	assert.NotContains(t, hooks, "SessionEnd")
	assert.NotContains(t, hooks, "SessionStart")
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

	// New hooks should be present (only PostToolUse — session lifecycle is plugin-handled).
	assert.Contains(t, string(hooks["PostToolUse"]), "claude-sync auto-commit --if-changed")
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

	// Only PostToolUse should have a hook rule entry.
	var rules []json.RawMessage
	require.NoError(t, json.Unmarshal(hooks["PostToolUse"], &rules))
	assert.Len(t, rules, 1, "PostToolUse should have exactly 1 rule after two calls")
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
	assert.Contains(t, desc, "plugin")
	assert.Contains(t, desc, "claude-sync")
}

func TestCleanupLegacyHooks_RemovesLegacyEntries(t *testing.T) {
	claudeDir := t.TempDir()

	// Simulate settings.json with legacy hooks from previous versions.
	settings := map[string]any{
		"model": "opus",
		"hooks": map[string]any{
			"SessionStart": []any{
				map[string]any{
					"matcher": "",
					"hooks": []any{
						map[string]any{"type": "command", "command": "bash ~/.claude/hooks/claude-sync-session-start.sh"},
					},
				},
			},
			"SessionEnd": []any{
				map[string]any{
					"matcher": "",
					"hooks": []any{
						map[string]any{"type": "command", "command": "bash ~/.claude/hooks/claude-sync-session-end.sh"},
					},
				},
			},
			"Stop": []any{
				map[string]any{
					"matcher": "",
					"hooks": []any{
						map[string]any{"type": "command", "command": "bash ~/.claude/hooks/stop-next-steps.sh"},
					},
				},
				map[string]any{
					"matcher": "",
					"hooks": []any{
						map[string]any{"type": "command", "command": "bash ~/.claude/hooks/claude-sync-stop-check.sh"},
					},
				},
			},
			"PreToolUse": []any{
				map[string]any{
					"matcher": "Bash",
					"hooks": []any{
						map[string]any{"type": "command", "command": "echo validator"},
					},
				},
			},
		},
	}
	data, err := json.MarshalIndent(settings, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(claudeDir, "settings.json"), data, 0644))

	err = CleanupLegacyHooks(claudeDir)
	require.NoError(t, err)

	data, err = os.ReadFile(filepath.Join(claudeDir, "settings.json"))
	require.NoError(t, err)

	var result map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(data, &result))

	var hooks map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(result["hooks"], &hooks))

	// Legacy hooks should be removed.
	_, hasSessionStart := hooks["SessionStart"]
	_, hasSessionEnd := hooks["SessionEnd"]
	_, hasStop := hooks["Stop"]
	assert.False(t, hasSessionStart, "SessionStart should be removed")
	assert.False(t, hasSessionEnd, "SessionEnd should be removed")
	assert.False(t, hasStop, "Stop should be removed (both entries were legacy)")

	// Non-legacy hooks should be preserved.
	assert.Contains(t, string(hooks["PreToolUse"]), "echo validator")

	// Model setting should be preserved.
	assert.Contains(t, string(result["model"]), "opus")
}

func TestCleanupLegacyHooks_PreservesNonLegacyStopHooks(t *testing.T) {
	claudeDir := t.TempDir()

	// Settings with a mix of legacy and non-legacy Stop hooks.
	settings := map[string]any{
		"hooks": map[string]any{
			"Stop": []any{
				map[string]any{
					"matcher": "",
					"hooks": []any{
						map[string]any{"type": "command", "command": "bash ~/.claude/hooks/stop-next-steps.sh"},
					},
				},
				map[string]any{
					"matcher": "",
					"hooks": []any{
						map[string]any{"type": "command", "command": "echo custom-stop-hook"},
					},
				},
			},
		},
	}
	data, err := json.MarshalIndent(settings, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(claudeDir, "settings.json"), data, 0644))

	err = CleanupLegacyHooks(claudeDir)
	require.NoError(t, err)

	data, err = os.ReadFile(filepath.Join(claudeDir, "settings.json"))
	require.NoError(t, err)

	var result map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(data, &result))

	var hooks map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(result["hooks"], &hooks))

	// Stop should still exist with the non-legacy hook preserved.
	assert.Contains(t, string(hooks["Stop"]), "echo custom-stop-hook")
	assert.NotContains(t, string(hooks["Stop"]), "stop-next-steps.sh")
}

func TestCleanupLegacyHooks_Idempotent(t *testing.T) {
	claudeDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(claudeDir, "settings.json"), []byte(`{"hooks":{}}`), 0644))

	// Should be no-op on empty hooks.
	err := CleanupLegacyHooks(claudeDir)
	require.NoError(t, err)

	// Call again — still no-op.
	err = CleanupLegacyHooks(claudeDir)
	require.NoError(t, err)
}

func TestCleanupLegacyHooks_SetupAutoSyncFormat(t *testing.T) {
	claudeDir := t.TempDir()

	// Simulate hooks from SetupAutoSyncHooks (old version that included SessionStart/SessionEnd).
	settings := map[string]any{
		"hooks": map[string]any{
			"SessionStart": []any{
				map[string]any{
					"matcher": "",
					"hooks": []any{
						map[string]any{"type": "command", "command": "claude-sync pull --auto"},
					},
				},
			},
			"SessionEnd": []any{
				map[string]any{
					"matcher": "",
					"hooks": []any{
						map[string]any{"type": "command", "command": "claude-sync push --auto --quiet"},
					},
				},
			},
			"PostToolUse": []any{
				map[string]any{
					"matcher": "Write|Edit",
					"hooks": []any{
						map[string]any{"type": "command", "command": "claude-sync auto-commit --if-changed"},
					},
				},
			},
		},
	}
	data, err := json.MarshalIndent(settings, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(claudeDir, "settings.json"), data, 0644))

	err = CleanupLegacyHooks(claudeDir)
	require.NoError(t, err)

	data, err = os.ReadFile(filepath.Join(claudeDir, "settings.json"))
	require.NoError(t, err)

	var result map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(data, &result))

	var hooks map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(result["hooks"], &hooks))

	// SessionStart and SessionEnd should be removed.
	_, hasSessionStart := hooks["SessionStart"]
	_, hasSessionEnd := hooks["SessionEnd"]
	assert.False(t, hasSessionStart)
	assert.False(t, hasSessionEnd)

	// PostToolUse should be preserved (not a legacy hook).
	assert.Contains(t, string(hooks["PostToolUse"]), "claude-sync auto-commit --if-changed")
}
