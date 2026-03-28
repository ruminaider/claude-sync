package commands_test

import (
	"strings"
	"testing"

	"github.com/ruminaider/claude-sync/internal/claudemd"
	"github.com/ruminaider/claude-sync/internal/commands"
	"github.com/stretchr/testify/assert"
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
