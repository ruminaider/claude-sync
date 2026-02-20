package commands_test

import (
	"encoding/json"
	"testing"

	"github.com/ruminaider/claude-sync/internal/commands"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSaveAndListConflicts(t *testing.T) {
	syncDir := t.TempDir()

	conflicts := []commands.PendingConflict{
		{
			Key:         "permissions.allow",
			LocalValue:  json.RawMessage(`["Read","Edit"]`),
			RemoteValue: json.RawMessage(`["Read","Bash(ls *)"]`),
		},
		{
			Key:         "settings.defaultMode",
			LocalValue:  json.RawMessage(`"plan"`),
			RemoteValue: json.RawMessage(`"acceptEdits"`),
		},
	}

	err := commands.SaveConflicts(syncDir, conflicts)
	require.NoError(t, err)

	assert.True(t, commands.HasPendingConflicts(syncDir))

	listed, err := commands.ListPendingConflicts(syncDir)
	require.NoError(t, err)
	assert.Len(t, listed, 2)
	assert.Equal(t, "permissions.allow", listed[0].Key)
}

func TestDiscardConflicts(t *testing.T) {
	syncDir := t.TempDir()

	conflicts := []commands.PendingConflict{
		{Key: "test", LocalValue: json.RawMessage(`"a"`), RemoteValue: json.RawMessage(`"b"`)},
	}
	commands.SaveConflicts(syncDir, conflicts)
	assert.True(t, commands.HasPendingConflicts(syncDir))

	err := commands.DiscardConflicts(syncDir)
	require.NoError(t, err)
	assert.False(t, commands.HasPendingConflicts(syncDir))
}

func TestHasPendingConflicts_Empty(t *testing.T) {
	syncDir := t.TempDir()
	assert.False(t, commands.HasPendingConflicts(syncDir))
}

func TestResolveConflict(t *testing.T) {
	syncDir := t.TempDir()

	conflicts := []commands.PendingConflict{
		{Key: "a", LocalValue: json.RawMessage(`"1"`), RemoteValue: json.RawMessage(`"2"`)},
		{Key: "b", LocalValue: json.RawMessage(`"3"`), RemoteValue: json.RawMessage(`"4"`)},
	}
	commands.SaveConflicts(syncDir, conflicts)

	err := commands.ResolveConflict(syncDir, 0, "local")
	require.NoError(t, err)

	remaining, _ := commands.ListPendingConflicts(syncDir)
	assert.Len(t, remaining, 1)
	assert.Equal(t, "b", remaining[0].Key)
}

func TestPushBlockedByConflicts(t *testing.T) {
	syncDir := t.TempDir()

	// Save a conflict
	commands.SaveConflicts(syncDir, []commands.PendingConflict{
		{Key: "test", LocalValue: json.RawMessage(`"a"`), RemoteValue: json.RawMessage(`"b"`)},
	})

	// HasPendingConflicts should be true
	assert.True(t, commands.HasPendingConflicts(syncDir))
	// In a real push flow, this would block the push
}
