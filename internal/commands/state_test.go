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

func TestDetectMenuState_WithPlugins(t *testing.T) {
	claudeDir := t.TempDir()
	syncDir := t.TempDir()

	// Config with upstream, pinned, and forked plugins
	cfgYAML := `version: "1.0.0"
plugins:
  upstream:
    - beads@beads-marketplace
    - hookify@claude-plugins-official
  pinned:
    - superpowers@claude-plugins-official: "1.2.3"
  forked:
    - my-tool
`
	require.NoError(t, os.WriteFile(filepath.Join(syncDir, "config.yaml"), []byte(cfgYAML), 0644))

	state := DetectMenuState(claudeDir, syncDir)

	require.Len(t, state.Plugins, 4)

	// Build a map for easier assertions
	byKey := map[string]PluginInfo{}
	for _, p := range state.Plugins {
		byKey[p.Key] = p
	}

	// upstream
	assert.Equal(t, "upstream", byKey["beads@beads-marketplace"].Status)
	assert.Equal(t, "beads", byKey["beads@beads-marketplace"].Name)

	assert.Equal(t, "upstream", byKey["hookify@claude-plugins-official"].Status)
	assert.Equal(t, "hookify", byKey["hookify@claude-plugins-official"].Name)

	// pinned
	assert.Equal(t, "pinned", byKey["superpowers@claude-plugins-official"].Status)
	assert.Equal(t, "1.2.3", byKey["superpowers@claude-plugins-official"].PinVersion)

	// forked — key includes @claude-sync-forks
	assert.Equal(t, "forked", byKey["my-tool@claude-sync-forks"].Status)
	assert.Equal(t, "my-tool", byKey["my-tool@claude-sync-forks"].Name)
}

func TestDetectMenuState_WithProjects(t *testing.T) {
	claudeDir := t.TempDir()
	syncDir := t.TempDir()

	require.NoError(t, os.WriteFile(filepath.Join(syncDir, "config.yaml"), []byte("version: \"1.0.0\"\nplugins:\n  upstream: []\n"), 0644))

	// Create two fake projects under a parent dir
	parentDir := t.TempDir()
	proj1 := filepath.Join(parentDir, "project-a")
	proj2 := filepath.Join(parentDir, "project-b")

	for _, dir := range []string{proj1, proj2} {
		require.NoError(t, os.MkdirAll(filepath.Join(dir, ".claude"), 0755))
	}
	require.NoError(t, os.WriteFile(filepath.Join(proj1, ".claude", ".claude-sync.yaml"),
		[]byte("version: \"1.0.0\"\nprofile: work\n"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(proj2, ".claude", ".claude-sync.yaml"),
		[]byte("version: \"1.0.0\"\nprofile: \"\"\n"), 0644))

	// Override DefaultProjectSearchDirs for test
	origFn := DefaultProjectSearchDirs
	DefaultProjectSearchDirs = func() []string { return []string{parentDir} }
	defer func() { DefaultProjectSearchDirs = origFn }()

	state := DetectMenuState(claudeDir, syncDir)

	require.Len(t, state.Projects, 2)

	// Build map by path
	byPath := map[string]ProjectInfo{}
	for _, p := range state.Projects {
		byPath[p.Path] = p
	}

	assert.Equal(t, "work", byPath[proj1].Profile)
	assert.Equal(t, "", byPath[proj2].Profile)
}

func TestDetectMenuState_EmptyConfig_NoPluginsOrProjects(t *testing.T) {
	claudeDir := t.TempDir()
	syncDir := t.TempDir()

	// Empty plugins
	require.NoError(t, os.WriteFile(filepath.Join(syncDir, "config.yaml"),
		[]byte("version: \"1.0.0\"\nplugins:\n"), 0644))

	// Override project dirs to an empty temp dir with no projects
	origFn := DefaultProjectSearchDirs
	DefaultProjectSearchDirs = func() []string { return []string{t.TempDir()} }
	defer func() { DefaultProjectSearchDirs = origFn }()

	state := DetectMenuState(claudeDir, syncDir)

	assert.Empty(t, state.Plugins)
	assert.Empty(t, state.Projects)
}
