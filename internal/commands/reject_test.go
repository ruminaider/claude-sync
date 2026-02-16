package commands_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ruminaider/claude-sync/internal/approval"
	"github.com/ruminaider/claude-sync/internal/commands"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRejectClears(t *testing.T) {
	syncDir := t.TempDir()

	pending := approval.PendingChanges{
		Commit: "abc123",
		Permissions: &approval.PendingPermissions{
			Allow: []string{"Bash(git *)"},
		},
	}
	require.NoError(t, approval.WritePending(syncDir, pending))

	// Verify file exists.
	_, err := os.Stat(filepath.Join(syncDir, "pending-changes.yaml"))
	require.NoError(t, err)

	err = commands.Reject(syncDir)
	require.NoError(t, err)

	// Verify file is gone.
	_, err = os.Stat(filepath.Join(syncDir, "pending-changes.yaml"))
	assert.True(t, os.IsNotExist(err))
}

func TestRejectNoPending(t *testing.T) {
	syncDir := t.TempDir()

	err := commands.Reject(syncDir)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no pending changes to reject")
}
