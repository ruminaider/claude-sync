package commands

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDetectMenuState_NotInitialized(t *testing.T) {
	claudeDir := t.TempDir()
	syncDir := filepath.Join(t.TempDir(), "sync")
	// syncDir does not exist

	state := DetectMenuState(claudeDir, syncDir)

	assert.False(t, state.ConfigExists)
	assert.False(t, state.HasPending)
	assert.False(t, state.HasConflicts)
	assert.Empty(t, state.Profiles)
	assert.Empty(t, state.ActiveProfile)
}

func TestDetectMenuState_Initialized(t *testing.T) {
	claudeDir := t.TempDir()
	syncDir := t.TempDir()

	// Create a minimal config.yaml so it looks initialized
	err := os.WriteFile(filepath.Join(syncDir, "config.yaml"), []byte("version: \"1.0.0\"\nplugins: []\n"), 0644)
	require.NoError(t, err)

	state := DetectMenuState(claudeDir, syncDir)

	assert.True(t, state.ConfigExists)
}

func TestDetectMenuState_WithProfiles(t *testing.T) {
	claudeDir := t.TempDir()
	syncDir := t.TempDir()

	err := os.WriteFile(filepath.Join(syncDir, "config.yaml"), []byte("version: \"1.0.0\"\nplugins: []\n"), 0644)
	require.NoError(t, err)

	// Create profiles directory with two profiles
	profilesDir := filepath.Join(syncDir, "profiles")
	require.NoError(t, os.MkdirAll(profilesDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(profilesDir, "work.yaml"), []byte("plugins: {}\n"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(profilesDir, "personal.yaml"), []byte("plugins: {}\n"), 0644))

	// Set active profile
	require.NoError(t, os.WriteFile(filepath.Join(syncDir, "active-profile"), []byte("work"), 0644))

	state := DetectMenuState(claudeDir, syncDir)

	assert.True(t, state.ConfigExists)
	assert.Contains(t, state.Profiles, "work")
	assert.Contains(t, state.Profiles, "personal")
	assert.Equal(t, "work", state.ActiveProfile)
}

func TestDetectMenuState_WithPendingChanges(t *testing.T) {
	claudeDir := t.TempDir()
	syncDir := t.TempDir()

	err := os.WriteFile(filepath.Join(syncDir, "config.yaml"), []byte("version: \"1.0.0\"\nplugins: []\n"), 0644)
	require.NoError(t, err)

	// Create a non-empty pending-changes.yaml
	pendingContent := "pending_since: \"2026-02-26\"\ncommit: \"abc123\"\npermissions:\n  allow:\n    - Bash\n"
	require.NoError(t, os.WriteFile(filepath.Join(syncDir, "pending-changes.yaml"), []byte(pendingContent), 0644))

	state := DetectMenuState(claudeDir, syncDir)

	assert.True(t, state.HasPending)
}

func TestDetectMenuState_WithConflicts(t *testing.T) {
	claudeDir := t.TempDir()
	syncDir := t.TempDir()

	err := os.WriteFile(filepath.Join(syncDir, "config.yaml"), []byte("version: \"1.0.0\"\nplugins: []\n"), 0644)
	require.NoError(t, err)

	// Create conflicts directory with a conflict file
	conflictsDir := filepath.Join(syncDir, "conflicts")
	require.NoError(t, os.MkdirAll(conflictsDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(conflictsDir, "1234.yaml"), []byte("key: test\n"), 0644))

	state := DetectMenuState(claudeDir, syncDir)

	assert.True(t, state.HasConflicts)
}
