package commands_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ruminaider/claude-sync/internal/claudemd"
	"github.com/ruminaider/claude-sync/internal/commands"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFormatPullPreview_NothingToChange(t *testing.T) {
	p := &commands.PullPreviewResult{NothingToChange: true}
	result := commands.FormatPullPreview(p)
	assert.Contains(t, result, "up to date")
}

func TestFormatPullPreview_WithChanges(t *testing.T) {
	p := &commands.PullPreviewResult{
		CommitsBehind:   3,
		ChangedFiles:    []string{"config.yaml", "claude-md/git-commits.md"},
		SettingsChanged: true,
		ClaudeMDChanged: true,
	}
	result := commands.FormatPullPreview(p)
	assert.Contains(t, result, "3 new commit(s)")
	assert.Contains(t, result, "config/settings")
	assert.Contains(t, result, "CLAUDE.md")
	assert.Contains(t, result, "config.yaml")
	assert.Contains(t, result, "claude-md/git-commits.md")
}

func TestFormatPullPreview_ManyFiles(t *testing.T) {
	files := make([]string, 15)
	for i := range files {
		files[i] = "file" + string(rune('a'+i))
	}
	p := &commands.PullPreviewResult{
		CommitsBehind: 1,
		ChangedFiles:  files,
	}
	result := commands.FormatPullPreview(p)
	assert.Contains(t, result, "15 changed")
	// Should not list individual files when > 10
	assert.NotContains(t, result, "filea")
}

func TestFormatPullPreview_AllCategories(t *testing.T) {
	p := &commands.PullPreviewResult{
		CommitsBehind:   2,
		ChangedFiles:    []string{"config.yaml", "claude-md/x.md", "commands/y.md", "skills/z/SKILL.md", "memory/m.json"},
		SettingsChanged: true,
		ClaudeMDChanged: true,
		CommandsChanged: true,
		SkillsChanged:   true,
		MemoryChanged:   true,
	}
	result := commands.FormatPullPreview(p)
	assert.Contains(t, result, "config/settings")
	assert.Contains(t, result, "CLAUDE.md")
	assert.Contains(t, result, "commands")
	assert.Contains(t, result, "skills")
	assert.Contains(t, result, "memory")
}

func TestPushPreviewSummary_NilScan(t *testing.T) {
	result := commands.PushPreviewSummary(nil)
	assert.Contains(t, result, "Nothing to push")
}

func TestPushPreviewSummary_NoChanges(t *testing.T) {
	scan := &commands.PushScanResult{}
	result := commands.PushPreviewSummary(scan)
	assert.Contains(t, result, "Nothing to push")
}

func TestPushPreviewSummary_PluginsAdded(t *testing.T) {
	scan := &commands.PushScanResult{
		AddedPlugins: []string{"plugin-a@marketplace", "plugin-b@marketplace"},
	}
	result := commands.PushPreviewSummary(scan)
	assert.Contains(t, result, "2 plugin(s) to add")
	assert.Contains(t, result, "+ plugin-a@marketplace")
	assert.Contains(t, result, "+ plugin-b@marketplace")
}

func TestPushPreviewSummary_PluginsRemoved(t *testing.T) {
	scan := &commands.PushScanResult{
		RemovedPlugins: []string{"old-plugin@marketplace"},
	}
	result := commands.PushPreviewSummary(scan)
	assert.Contains(t, result, "1 plugin(s) to remove")
	assert.Contains(t, result, "- old-plugin@marketplace")
}

func TestPushPreviewSummary_ConfigChanges(t *testing.T) {
	scan := &commands.PushScanResult{
		ChangedPermissions: true,
		ChangedClaudeMD:    &claudemd.ReconcileResult{Updated: []string{"x"}},
		ChangedMCP:         true,
		ChangedKeybindings: true,
		ChangedCommands:    true,
		ChangedSkills:      true,
	}
	result := commands.PushPreviewSummary(scan)
	assert.Contains(t, result, "permissions changed")
	assert.Contains(t, result, "CLAUDE.md changed")
	assert.Contains(t, result, "MCP servers changed")
	assert.Contains(t, result, "keybindings changed")
	assert.Contains(t, result, "commands changed")
	assert.Contains(t, result, "skills changed")
}

func TestPushPreviewSummary_OrphansAndDirty(t *testing.T) {
	scan := &commands.PushScanResult{
		OrphanedCommands: []string{"old-cmd"},
		OrphanedSkills:   []string{"old-skill"},
		DirtyWorkingTree: true,
	}
	result := commands.PushPreviewSummary(scan)
	assert.Contains(t, result, "1 orphaned command(s)")
	assert.Contains(t, result, "1 orphaned skill(s)")
	assert.Contains(t, result, "uncommitted config changes")
}

func TestPushPreviewSummary_CombinedChanges(t *testing.T) {
	scan := &commands.PushScanResult{
		AddedPlugins:       []string{"new-plugin"},
		ChangedPermissions: true,
		DirtyWorkingTree:   true,
	}
	result := commands.PushPreviewSummary(scan)
	lines := strings.Split(result, "\n")
	assert.True(t, len(lines) >= 3, "expected at least 3 lines, got %d", len(lines))
}

// ---------------------------------------------------------------------------
// PullPreview integration tests
// ---------------------------------------------------------------------------

// setupBareClone creates a bare remote repo, clones it, makes an initial commit
// with config.yaml, and pushes. It returns (remoteDir, cloneDir).
func setupBareClone(t *testing.T) (string, string) {
	t.Helper()

	remote := t.TempDir()
	require.NoError(t, exec.Command("git", "init", "--bare", remote).Run())

	cloneDir := filepath.Join(t.TempDir(), "sync")
	require.NoError(t, exec.Command("git", "clone", remote, cloneDir).Run())

	require.NoError(t, exec.Command("git", "-C", cloneDir, "config", "user.email", "test@test.com").Run())
	require.NoError(t, exec.Command("git", "-C", cloneDir, "config", "user.name", "Test").Run())

	require.NoError(t, os.WriteFile(filepath.Join(cloneDir, "config.yaml"), []byte("version: \"1.0.0\"\n"), 0644))
	require.NoError(t, exec.Command("git", "-C", cloneDir, "add", ".").Run())
	require.NoError(t, exec.Command("git", "-C", cloneDir, "commit", "-m", "init").Run())
	require.NoError(t, exec.Command("git", "-C", cloneDir, "push", "origin", "HEAD").Run())

	return remote, cloneDir
}

// addRemoteCommit clones the bare remote into a temporary directory, creates a
// commit that adds/modifies the given file with the given content, and pushes.
func addRemoteCommit(t *testing.T, remote, filePath, content, msg string) {
	t.Helper()

	otherClone := filepath.Join(t.TempDir(), "other")
	require.NoError(t, exec.Command("git", "clone", remote, otherClone).Run())
	require.NoError(t, exec.Command("git", "-C", otherClone, "config", "user.email", "test@test.com").Run())
	require.NoError(t, exec.Command("git", "-C", otherClone, "config", "user.name", "Test").Run())

	fullPath := filepath.Join(otherClone, filePath)
	require.NoError(t, os.MkdirAll(filepath.Dir(fullPath), 0755))
	require.NoError(t, os.WriteFile(fullPath, []byte(content), 0644))
	require.NoError(t, exec.Command("git", "-C", otherClone, "add", ".").Run())
	require.NoError(t, exec.Command("git", "-C", otherClone, "commit", "-m", msg).Run())
	require.NoError(t, exec.Command("git", "-C", otherClone, "push", "origin", "HEAD").Run())
}

func TestPullPreview_NothingToChange(t *testing.T) {
	_, cloneDir := setupBareClone(t)

	// Fetch to make sure tracking refs are current (already up to date).
	require.NoError(t, exec.Command("git", "-C", cloneDir, "fetch").Run())

	preview, err := commands.PullPreview(cloneDir)
	require.NoError(t, err)
	assert.True(t, preview.NothingToChange, "expected NothingToChange when local == upstream")
}

func TestPullPreview_DetectsIncomingCommits(t *testing.T) {
	remote, cloneDir := setupBareClone(t)

	addRemoteCommit(t, remote, "extra.txt", "new content", "add extra")

	require.NoError(t, exec.Command("git", "-C", cloneDir, "fetch").Run())

	preview, err := commands.PullPreview(cloneDir)
	require.NoError(t, err)
	assert.False(t, preview.NothingToChange)
	assert.Greater(t, preview.CommitsBehind, 0, "expected at least one incoming commit")
}

func TestPullPreview_CategorizesConfigYaml(t *testing.T) {
	remote, cloneDir := setupBareClone(t)

	addRemoteCommit(t, remote, "config.yaml", "version: \"2.0.0\"\n", "update config")

	require.NoError(t, exec.Command("git", "-C", cloneDir, "fetch").Run())

	preview, err := commands.PullPreview(cloneDir)
	require.NoError(t, err)
	assert.True(t, preview.SettingsChanged, "expected SettingsChanged for config.yaml")
}

func TestPullPreview_CategorizesClaudeMD(t *testing.T) {
	remote, cloneDir := setupBareClone(t)

	addRemoteCommit(t, remote, "claude-md/coding-standards.md", "# Standards\n", "add claude-md")

	require.NoError(t, exec.Command("git", "-C", cloneDir, "fetch").Run())

	preview, err := commands.PullPreview(cloneDir)
	require.NoError(t, err)
	assert.True(t, preview.ClaudeMDChanged, "expected ClaudeMDChanged for claude-md/ file")
}

func TestPullPreview_CategorizesSkills(t *testing.T) {
	remote, cloneDir := setupBareClone(t)

	addRemoteCommit(t, remote, "skills/my-skill/SKILL.md", "# Skill\n", "add skill")

	require.NoError(t, exec.Command("git", "-C", cloneDir, "fetch").Run())

	preview, err := commands.PullPreview(cloneDir)
	require.NoError(t, err)
	assert.True(t, preview.SkillsChanged, "expected SkillsChanged for skills/ file")
}

func TestPullPreview_CountsPendingCommits(t *testing.T) {
	remote, cloneDir := setupBareClone(t)

	addRemoteCommit(t, remote, "file1.txt", "one", "commit 1")
	addRemoteCommit(t, remote, "file2.txt", "two", "commit 2")
	addRemoteCommit(t, remote, "file3.txt", "three", "commit 3")

	require.NoError(t, exec.Command("git", "-C", cloneDir, "fetch").Run())

	preview, err := commands.PullPreview(cloneDir)
	require.NoError(t, err)
	assert.Equal(t, 3, preview.CommitsBehind, "expected exactly 3 pending commits")
}
