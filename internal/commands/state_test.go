package commands

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/ruminaider/claude-sync/internal/project"
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

	cfgYAML := `version: "1.0.0"
plugins:
  upstream:
    - auto-compact@claude-plugins-official
    - memory@claude-plugins-official
  pinned:
    - custom-tool@my-marketplace: "v1.2.3"
  forked:
    - my-fork
`
	require.NoError(t, os.WriteFile(filepath.Join(syncDir, "config.yaml"), []byte(cfgYAML), 0644))

	state := DetectMenuState(claudeDir, syncDir)

	require.Len(t, state.Plugins, 4) // 2 upstream + 1 pinned + 1 forked

	// Build a map by key for easier assertions
	byKey := make(map[string]PluginInfo)
	for _, p := range state.Plugins {
		byKey[p.Key] = p
	}

	// Upstream plugins
	up1 := byKey["auto-compact@claude-plugins-official"]
	assert.Equal(t, "auto-compact", up1.Name)
	assert.Equal(t, "upstream", up1.Status)
	assert.Equal(t, "claude-plugins-official", up1.Marketplace)

	up2 := byKey["memory@claude-plugins-official"]
	assert.Equal(t, "memory", up2.Name)
	assert.Equal(t, "upstream", up2.Status)

	// Pinned plugin
	pinned := byKey["custom-tool@my-marketplace"]
	assert.Equal(t, "custom-tool", pinned.Name)
	assert.Equal(t, "pinned", pinned.Status)
	assert.Equal(t, "v1.2.3", pinned.PinVersion)
	assert.Equal(t, "my-marketplace", pinned.Marketplace)

	// Forked plugin
	forked := byKey["my-fork@claude-sync-forks"]
	assert.Equal(t, "my-fork", forked.Name)
	assert.Equal(t, "forked", forked.Status)
	assert.Equal(t, "claude-sync-forks", forked.Marketplace)
}

func TestDetectMenuState_WithProjects_Dashboard(t *testing.T) {
	claudeDir := t.TempDir()
	syncDir := t.TempDir()

	require.NoError(t, os.WriteFile(filepath.Join(syncDir, "config.yaml"), []byte("version: \"1.0.0\"\n"), 0644))

	// Create parent dir with two project subdirectories
	parent := t.TempDir()
	proj1 := filepath.Join(parent, "project-a")
	proj2 := filepath.Join(parent, "project-b")
	require.NoError(t, os.MkdirAll(filepath.Join(proj1, ".claude"), 0755))
	require.NoError(t, os.MkdirAll(filepath.Join(proj2, ".claude"), 0755))

	require.NoError(t, project.WriteProjectConfig(proj1, project.ProjectConfig{Version: "1.0.0", Profile: "work"}))
	require.NoError(t, project.WriteProjectConfig(proj2, project.ProjectConfig{Version: "1.0.0", Profile: "personal"}))

	// Override DefaultProjectSearchDirs for test
	origSearchDirs := DefaultProjectSearchDirs
	DefaultProjectSearchDirs = func() []string { return []string{parent} }
	defer func() { DefaultProjectSearchDirs = origSearchDirs }()

	state := DetectMenuState(claudeDir, syncDir)

	require.Len(t, state.Projects, 2)
	byPath := make(map[string]ProjectInfo)
	for _, p := range state.Projects {
		byPath[p.Path] = p
	}
	assert.Equal(t, "work", byPath[proj1].Profile)
	assert.Equal(t, "personal", byPath[proj2].Profile)
}

func TestDetectMenuState_ConfigRepo(t *testing.T) {
	claudeDir := t.TempDir()
	syncDir := t.TempDir()

	require.NoError(t, os.WriteFile(filepath.Join(syncDir, "config.yaml"), []byte("version: \"1.0.0\"\n"), 0644))

	// Initialize a git repo with a remote
	require.NoError(t, exec.Command("git", "init", syncDir).Run())
	require.NoError(t, exec.Command("git", "-C", syncDir, "remote", "add", "origin", "https://github.com/ruminaider/claude-sync-config.git").Run())

	state := DetectMenuState(claudeDir, syncDir)

	assert.Equal(t, "ruminaider/claude-sync-config", state.ConfigRepo)
}

func TestDetectMenuState_ConfigRepo_SSH(t *testing.T) {
	claudeDir := t.TempDir()
	syncDir := t.TempDir()

	require.NoError(t, os.WriteFile(filepath.Join(syncDir, "config.yaml"), []byte("version: \"1.0.0\"\n"), 0644))

	require.NoError(t, exec.Command("git", "init", syncDir).Run())
	require.NoError(t, exec.Command("git", "-C", syncDir, "remote", "add", "origin", "git@github.com:ruminaider/claude-sync-config.git").Run())

	state := DetectMenuState(claudeDir, syncDir)

	assert.Equal(t, "ruminaider/claude-sync-config", state.ConfigRepo)
}

func TestDetectMenuState_ConfigRepo_NoGit(t *testing.T) {
	claudeDir := t.TempDir()
	syncDir := t.TempDir()

	require.NoError(t, os.WriteFile(filepath.Join(syncDir, "config.yaml"), []byte("version: \"1.0.0\"\n"), 0644))

	// No git repo — ConfigRepo should be empty
	state := DetectMenuState(claudeDir, syncDir)

	assert.Empty(t, state.ConfigRepo)
}

func TestDetectMenuState_ProjectDir(t *testing.T) {
	claudeDir := t.TempDir()
	syncDir := t.TempDir()

	require.NoError(t, os.WriteFile(filepath.Join(syncDir, "config.yaml"), []byte("version: \"1.0.0\"\n"), 0644))

	// Create a project in a temp dir and simulate being "in" it
	projDir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(projDir, ".claude"), 0755))
	require.NoError(t, project.WriteProjectConfig(projDir, project.ProjectConfig{
		Version: "1.0.0",
		Profile: "work",
	}))

	// Override DefaultProjectSearchDirs to avoid scanning real dirs
	origSearchDirs := DefaultProjectSearchDirs
	DefaultProjectSearchDirs = func() []string { return nil }
	defer func() { DefaultProjectSearchDirs = origSearchDirs }()

	// Use the overload that accepts pwd
	state := detectMenuStateWithPwd(claudeDir, syncDir, projDir)

	assert.Equal(t, projDir, state.ProjectDir)
	assert.Equal(t, "work", state.ProjectProfile)
}

func TestDetectMenuState_ProjectDir_NotManaged(t *testing.T) {
	claudeDir := t.TempDir()
	syncDir := t.TempDir()

	require.NoError(t, os.WriteFile(filepath.Join(syncDir, "config.yaml"), []byte("version: \"1.0.0\"\n"), 0644))

	projDir := t.TempDir()
	// No .claude/.claude-sync.yaml

	origSearchDirs := DefaultProjectSearchDirs
	DefaultProjectSearchDirs = func() []string { return nil }
	defer func() { DefaultProjectSearchDirs = origSearchDirs }()

	state := detectMenuStateWithPwd(claudeDir, syncDir, projDir)

	assert.Empty(t, state.ProjectDir)
	assert.Empty(t, state.ProjectProfile)
}

func TestDetectMenuState_ClaudeMDCount(t *testing.T) {
	claudeDir := t.TempDir()
	syncDir := t.TempDir()

	require.NoError(t, os.WriteFile(filepath.Join(syncDir, "config.yaml"), []byte("version: \"1.0.0\"\n"), 0644))

	// Create claude-md directory with fragments
	claudeMDDir := filepath.Join(syncDir, "claude-md")
	require.NoError(t, os.MkdirAll(claudeMDDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(claudeMDDir, "coding-standards.md"), []byte("# Coding Standards\n"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(claudeMDDir, "project-context.md"), []byte("# Context\n"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(claudeMDDir, "manifest.yaml"), []byte("files:\n"), 0644)) // not a .md file

	state := DetectMenuState(claudeDir, syncDir)

	assert.Equal(t, 2, state.ClaudeMDCount)
}

func TestDetectMenuState_MCPCount(t *testing.T) {
	claudeDir := t.TempDir()
	syncDir := t.TempDir()

	cfgYAML := `version: "1.0.0"
mcp:
  slack:
    command: mcp-slack
  notion:
    command: mcp-notion
  github:
    command: mcp-github
`
	require.NoError(t, os.WriteFile(filepath.Join(syncDir, "config.yaml"), []byte(cfgYAML), 0644))

	state := DetectMenuState(claudeDir, syncDir)

	assert.Equal(t, 3, state.MCPCount)
}

func TestDetectMenuState_EmptyConfig(t *testing.T) {
	claudeDir := t.TempDir()
	syncDir := t.TempDir()

	require.NoError(t, os.WriteFile(filepath.Join(syncDir, "config.yaml"), []byte("version: \"1.0.0\"\n"), 0644))

	// Override DefaultProjectSearchDirs to avoid scanning real dirs
	origSearchDirs := DefaultProjectSearchDirs
	DefaultProjectSearchDirs = func() []string { return nil }
	defer func() { DefaultProjectSearchDirs = origSearchDirs }()

	state := DetectMenuState(claudeDir, syncDir)

	assert.True(t, state.ConfigExists)
	assert.Empty(t, state.ConfigRepo)
	assert.Equal(t, 0, state.CommitsBehind)
	assert.Empty(t, state.Plugins)
	assert.Empty(t, state.Projects)
	assert.Empty(t, state.ProjectDir)
	assert.Empty(t, state.ProjectProfile)
	assert.Equal(t, 0, state.ClaudeMDCount)
	assert.Equal(t, 0, state.MCPCount)
}

func TestDetectMenuState_CommitsBehind(t *testing.T) {
	claudeDir := t.TempDir()

	// Create a bare remote repo
	remote := t.TempDir()
	require.NoError(t, exec.Command("git", "init", "--bare", remote).Run())

	// Clone it to syncDir
	syncDir := filepath.Join(t.TempDir(), "sync")
	require.NoError(t, exec.Command("git", "clone", remote, syncDir).Run())

	// Configure git user for commits
	require.NoError(t, exec.Command("git", "-C", syncDir, "config", "user.email", "test@test.com").Run())
	require.NoError(t, exec.Command("git", "-C", syncDir, "config", "user.name", "Test").Run())

	// Create config.yaml and commit
	require.NoError(t, os.WriteFile(filepath.Join(syncDir, "config.yaml"), []byte("version: \"1.0.0\"\n"), 0644))
	require.NoError(t, exec.Command("git", "-C", syncDir, "add", ".").Run())
	require.NoError(t, exec.Command("git", "-C", syncDir, "commit", "-m", "init").Run())
	require.NoError(t, exec.Command("git", "-C", syncDir, "push", "origin", "HEAD").Run())

	// Make an additional commit directly on the remote (simulating someone else pushing)
	// by cloning again, committing, and pushing
	otherClone := filepath.Join(t.TempDir(), "other")
	require.NoError(t, exec.Command("git", "clone", remote, otherClone).Run())
	require.NoError(t, exec.Command("git", "-C", otherClone, "config", "user.email", "test@test.com").Run())
	require.NoError(t, exec.Command("git", "-C", otherClone, "config", "user.name", "Test").Run())
	require.NoError(t, os.WriteFile(filepath.Join(otherClone, "extra.txt"), []byte("new content"), 0644))
	require.NoError(t, exec.Command("git", "-C", otherClone, "add", ".").Run())
	require.NoError(t, exec.Command("git", "-C", otherClone, "commit", "-m", "extra").Run())
	require.NoError(t, exec.Command("git", "-C", otherClone, "push", "origin", "HEAD").Run())

	// Fetch in our syncDir so we know about the remote commit (but don't merge)
	require.NoError(t, exec.Command("git", "-C", syncDir, "fetch").Run())

	// Override DefaultProjectSearchDirs to avoid scanning real dirs
	origSearchDirs := DefaultProjectSearchDirs
	DefaultProjectSearchDirs = func() []string { return nil }
	defer func() { DefaultProjectSearchDirs = origSearchDirs }()

	state := DetectMenuState(claudeDir, syncDir)

	assert.Equal(t, 1, state.CommitsBehind)
}
