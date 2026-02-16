package approval_test

import (
	"encoding/json"
	"testing"

	"github.com/ruminaider/claude-sync/internal/approval"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClassify_PermissionsAreHighRisk(t *testing.T) {
	changes := approval.ConfigChanges{
		Permissions: &approval.PermissionChanges{
			Allow: []string{"Bash(*)"},
		},
	}
	result := approval.Classify(changes)
	assert.Empty(t, result.Safe)
	require.Len(t, result.HighRisk, 1)
	assert.Equal(t, "permissions", result.HighRisk[0].Category)
}

func TestClassify_SettingsAreSafe(t *testing.T) {
	changes := approval.ConfigChanges{
		Settings: map[string]any{"theme": "dark"},
	}
	result := approval.Classify(changes)
	assert.Empty(t, result.HighRisk)
	require.Len(t, result.Safe, 1)
	assert.Equal(t, "settings", result.Safe[0].Category)
}

func TestClassify_HooksAreHighRisk(t *testing.T) {
	changes := approval.ConfigChanges{
		HasHookChanges: true,
	}
	result := approval.Classify(changes)
	assert.Empty(t, result.Safe)
	require.Len(t, result.HighRisk, 1)
	assert.Equal(t, "hooks", result.HighRisk[0].Category)
}

func TestClassify_MCPIsHighRisk(t *testing.T) {
	changes := approval.ConfigChanges{
		HasMCPChanges: true,
	}
	result := approval.Classify(changes)
	assert.Empty(t, result.Safe)
	require.Len(t, result.HighRisk, 1)
	assert.Equal(t, "mcp", result.HighRisk[0].Category)
}

func TestClassify_ClaudeMDIsSafe(t *testing.T) {
	changes := approval.ConfigChanges{
		ClaudeMD: []string{"coding-standards"},
	}
	result := approval.Classify(changes)
	assert.Empty(t, result.HighRisk)
	require.Len(t, result.Safe, 1)
	assert.Equal(t, "claude_md", result.Safe[0].Category)
}

func TestClassify_KeybindingsAreSafe(t *testing.T) {
	changes := approval.ConfigChanges{
		Keybindings: true,
	}
	result := approval.Classify(changes)
	assert.Empty(t, result.HighRisk)
	require.Len(t, result.Safe, 1)
	assert.Equal(t, "keybindings", result.Safe[0].Category)
}

func TestClassify_MixedChanges(t *testing.T) {
	changes := approval.ConfigChanges{
		Settings:       map[string]any{"theme": "dark"},
		Permissions:    &approval.PermissionChanges{Allow: []string{"Bash(*)"}},
		HasHookChanges: true,
		ClaudeMD:       []string{"standards"},
		Keybindings:    true,
	}
	result := approval.Classify(changes)
	assert.Len(t, result.Safe, 3)    // settings + claude_md + keybindings
	assert.Len(t, result.HighRisk, 2) // permissions + hooks
}

func TestPendingChanges_WriteAndRead(t *testing.T) {
	dir := t.TempDir()
	pending := approval.PendingChanges{
		PendingSince: "2026-02-14T12:00:00Z",
		Commit:       "abc123",
		Permissions: &approval.PendingPermissions{
			Allow: []string{"Bash(*)"},
		},
		MCP: map[string]json.RawMessage{
			"context7": json.RawMessage(`{"type":"stdio"}`),
		},
		Hooks: map[string]json.RawMessage{
			"PreCompact": json.RawMessage(`[{"matcher":"","hooks":[{"type":"command","command":"test"}]}]`),
		},
	}

	err := approval.WritePending(dir, pending)
	require.NoError(t, err)

	loaded, err := approval.ReadPending(dir)
	require.NoError(t, err)
	assert.Equal(t, "abc123", loaded.Commit)
	assert.Equal(t, "2026-02-14T12:00:00Z", loaded.PendingSince)
	require.NotNil(t, loaded.Permissions)
	assert.Equal(t, []string{"Bash(*)"}, loaded.Permissions.Allow)
	assert.Contains(t, loaded.MCP, "context7")
	assert.Contains(t, loaded.Hooks, "PreCompact")
}

func TestPendingChanges_ReadMissing(t *testing.T) {
	dir := t.TempDir()
	loaded, err := approval.ReadPending(dir)
	require.NoError(t, err)
	assert.True(t, loaded.IsEmpty())
}

func TestPendingChanges_Clear(t *testing.T) {
	dir := t.TempDir()
	pending := approval.PendingChanges{
		Commit: "abc123",
		Permissions: &approval.PendingPermissions{
			Allow: []string{"Bash(*)"},
		},
	}
	err := approval.WritePending(dir, pending)
	require.NoError(t, err)

	err = approval.ClearPending(dir)
	require.NoError(t, err)

	loaded, err := approval.ReadPending(dir)
	require.NoError(t, err)
	assert.True(t, loaded.IsEmpty())
}

func TestPendingChanges_IsEmpty(t *testing.T) {
	t.Run("empty struct", func(t *testing.T) {
		p := approval.PendingChanges{}
		assert.True(t, p.IsEmpty())
	})

	t.Run("has commit", func(t *testing.T) {
		p := approval.PendingChanges{Commit: "abc"}
		assert.False(t, p.IsEmpty())
	})

	t.Run("has permissions", func(t *testing.T) {
		p := approval.PendingChanges{
			Permissions: &approval.PendingPermissions{Allow: []string{"x"}},
		}
		assert.False(t, p.IsEmpty())
	})

	t.Run("has MCP", func(t *testing.T) {
		p := approval.PendingChanges{
			MCP: map[string]json.RawMessage{"x": json.RawMessage(`{}`)},
		}
		assert.False(t, p.IsEmpty())
	})

	t.Run("has hooks", func(t *testing.T) {
		p := approval.PendingChanges{
			Hooks: map[string]json.RawMessage{"x": json.RawMessage(`{}`)},
		}
		assert.False(t, p.IsEmpty())
	})

	t.Run("nil permissions struct", func(t *testing.T) {
		p := approval.PendingChanges{Permissions: nil}
		assert.True(t, p.IsEmpty())
	})

	t.Run("empty permissions slices", func(t *testing.T) {
		p := approval.PendingChanges{
			Permissions: &approval.PendingPermissions{},
		}
		assert.True(t, p.IsEmpty())
	})
}
