package commands

import (
	"fmt"
	"strings"

	"github.com/ruminaider/claude-sync/internal/git"
)

// PullPreviewResult summarizes what a pull operation would change.
type PullPreviewResult struct {
	CommitsBehind    int
	ChangedFiles     []string // file paths changed between HEAD and upstream
	PluginsToInstall int
	PluginsToUpdate  int
	SettingsChanged  bool
	ClaudeMDChanged  bool
	PermChanged      bool
	MCPChanged       bool
	KeybindChanged   bool
	CommandsChanged  bool
	SkillsChanged    bool
	MemoryChanged    bool
	NothingToChange  bool
}

// PullPreview fetches remote changes and returns a summary of what would change
// without actually applying anything. The caller can display this and ask for
// confirmation before proceeding.
func PullPreview(syncDir string) (*PullPreviewResult, error) {
	result := &PullPreviewResult{}

	if !git.HasRemote(syncDir, "origin") {
		result.NothingToChange = true
		return result, nil
	}

	// Fetch remote changes without merging.
	if err := git.Fetch(syncDir); err != nil {
		return nil, fmt.Errorf("fetching remote: %w", err)
	}

	result.CommitsBehind = git.CommitsBehind(syncDir)
	if result.CommitsBehind <= 0 {
		result.NothingToChange = true
		return result, nil
	}

	// List files that differ between HEAD and upstream.
	changedFiles := git.DiffNameOnly(syncDir, "HEAD", "@{upstream}")
	result.ChangedFiles = changedFiles

	// Categorize changes by file type.
	for _, f := range changedFiles {
		switch {
		case f == "config.yaml":
			// config.yaml changes can mean plugins, settings, permissions, MCP, etc.
			// We mark settings as changed as a generic indicator.
			result.SettingsChanged = true
		case strings.HasPrefix(f, "claude-md/"):
			result.ClaudeMDChanged = true
		case strings.HasPrefix(f, "commands/"):
			result.CommandsChanged = true
		case strings.HasPrefix(f, "skills/"):
			result.SkillsChanged = true
		case strings.HasPrefix(f, "memory/"):
			result.MemoryChanged = true
		case strings.HasPrefix(f, "profiles/"):
			result.SettingsChanged = true
		}
	}

	return result, nil
}

// FormatPullPreview returns a human-readable summary of incoming pull changes.
func FormatPullPreview(p *PullPreviewResult) string {
	if p.NothingToChange {
		return "Already up to date. Nothing to pull."
	}

	var lines []string
	lines = append(lines, fmt.Sprintf("Remote has %d new commit(s).", p.CommitsBehind))

	var changes []string
	if p.SettingsChanged {
		changes = append(changes, "config/settings")
	}
	if p.ClaudeMDChanged {
		changes = append(changes, "CLAUDE.md")
	}
	if p.CommandsChanged {
		changes = append(changes, "commands")
	}
	if p.SkillsChanged {
		changes = append(changes, "skills")
	}
	if p.MemoryChanged {
		changes = append(changes, "memory")
	}

	if len(changes) > 0 {
		lines = append(lines, fmt.Sprintf("Changes: %s", strings.Join(changes, ", ")))
	}
	if len(p.ChangedFiles) > 0 && len(p.ChangedFiles) <= 10 {
		lines = append(lines, "Files:")
		for _, f := range p.ChangedFiles {
			lines = append(lines, fmt.Sprintf("  %s", f))
		}
	} else if len(p.ChangedFiles) > 10 {
		lines = append(lines, fmt.Sprintf("Files: %d changed", len(p.ChangedFiles)))
	}

	return strings.Join(lines, "\n")
}

// PushPreviewSummary returns a human-readable summary of what push would send.
func PushPreviewSummary(scan *PushScanResult) string {
	if scan == nil || !scan.HasChanges() {
		return "Nothing to push. Everything matches config."
	}

	var parts []string

	if len(scan.AddedPlugins) > 0 {
		parts = append(parts, fmt.Sprintf("%d plugin(s) to add", len(scan.AddedPlugins)))
		for _, p := range scan.AddedPlugins {
			parts = append(parts, fmt.Sprintf("  + %s", p))
		}
	}
	if len(scan.RemovedPlugins) > 0 {
		parts = append(parts, fmt.Sprintf("%d plugin(s) to remove", len(scan.RemovedPlugins)))
		for _, p := range scan.RemovedPlugins {
			parts = append(parts, fmt.Sprintf("  - %s", p))
		}
	}
	if scan.ChangedPermissions {
		parts = append(parts, "permissions changed")
	}
	if scan.ChangedClaudeMD != nil {
		parts = append(parts, "CLAUDE.md changed")
	}
	if scan.ChangedMCP {
		parts = append(parts, "MCP servers changed")
	}
	if scan.ChangedKeybindings {
		parts = append(parts, "keybindings changed")
	}
	if scan.ChangedCommands {
		parts = append(parts, "commands changed")
	}
	if scan.ChangedSkills {
		parts = append(parts, "skills changed")
	}
	if len(scan.OrphanedCommands) > 0 {
		parts = append(parts, fmt.Sprintf("%d orphaned command(s) to remove", len(scan.OrphanedCommands)))
	}
	if len(scan.OrphanedSkills) > 0 {
		parts = append(parts, fmt.Sprintf("%d orphaned skill(s) to remove", len(scan.OrphanedSkills)))
	}
	if scan.DirtyWorkingTree {
		parts = append(parts, "uncommitted config changes")
	}

	return strings.Join(parts, "\n")
}
