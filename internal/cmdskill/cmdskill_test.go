package cmdskill

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ─── ParseFrontmatter ──────────────────────────────────────────────────────

func TestParseFrontmatter(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		wantName string
		wantDesc string
	}{
		{
			name: "standard frontmatter",
			content: `---
name: brainstorming
description: "Explore user intent and requirements"
---

# Content here`,
			wantName: "brainstorming",
			wantDesc: "Explore user intent and requirements",
		},
		{
			name: "single-quoted values",
			content: `---
name: 'my-skill'
description: 'A great skill'
---
Body`,
			wantName: "my-skill",
			wantDesc: "A great skill",
		},
		{
			name: "unquoted values",
			content: `---
name: deploy
description: Deploy the application
---
`,
			wantName: "deploy",
			wantDesc: "Deploy the application",
		},
		{
			name: "no frontmatter",
			content: `# Just a markdown file
No frontmatter here.`,
			wantName: "",
			wantDesc: "",
		},
		{
			name: "empty content",
			content:  "",
			wantName: "",
			wantDesc: "",
		},
		{
			name: "frontmatter without name or description",
			content: `---
allowed-tools: Bash(git *)
---
# Command`,
			wantName: "",
			wantDesc: "",
		},
		{
			name: "unclosed frontmatter",
			content: `---
name: broken
description: never closed
`,
			wantName: "",
			wantDesc: "",
		},
		{
			name: "description only",
			content: `---
description: Create a git commit
---
`,
			wantName: "",
			wantDesc: "Create a git commit",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			name, desc := ParseFrontmatter(tt.content)
			assert.Equal(t, tt.wantName, name, "name")
			assert.Equal(t, tt.wantDesc, desc, "description")
		})
	}
}

// ─── Key ───────────────────────────────────────────────────────────────────

func TestKey(t *testing.T) {
	tests := []struct {
		name string
		item Item
		want string
	}{
		{
			name: "global command",
			item: Item{Name: "review-pr", Type: TypeCommand, Source: SourceGlobal, SourceLabel: "global"},
			want: "cmd:global:review-pr",
		},
		{
			name: "global skill",
			item: Item{Name: "brainstorming", Type: TypeSkill, Source: SourceGlobal, SourceLabel: "global"},
			want: "skill:global:brainstorming",
		},
		{
			name: "plugin command",
			item: Item{Name: "commit", Type: TypeCommand, Source: SourcePlugin, SourceLabel: "commit-commands"},
			want: "cmd:plugin:commit-commands:commit",
		},
		{
			name: "plugin skill",
			item: Item{Name: "tdd", Type: TypeSkill, Source: SourcePlugin, SourceLabel: "superpowers"},
			want: "skill:plugin:superpowers:tdd",
		},
		{
			name: "project command",
			item: Item{Name: "deploy", Type: TypeCommand, Source: SourceProject, SourceLabel: "myproject"},
			want: "cmd:project:myproject:deploy",
		},
		{
			name: "project skill",
			item: Item{Name: "onboard", Type: TypeSkill, Source: SourceProject, SourceLabel: "myproject"},
			want: "skill:project:myproject:onboard",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.item.Key())
		})
	}
}

// ─── ScanGlobal ────────────────────────────────────────────────────────────

func TestScanGlobal(t *testing.T) {
	claudeDir := t.TempDir()

	// Create commands.
	commandsDir := filepath.Join(claudeDir, "commands")
	require.NoError(t, os.MkdirAll(commandsDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(commandsDir, "review-pr.md"), []byte(`---
description: Review a pull request
---
# Review PR`), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(commandsDir, "deploy.md"), []byte(`# Deploy
No frontmatter here.`), 0644))

	// Non-.md file should be ignored.
	require.NoError(t, os.WriteFile(filepath.Join(commandsDir, "notes.txt"), []byte("ignore me"), 0644))

	// Create skills.
	skillDir := filepath.Join(claudeDir, "skills", "brainstorming")
	require.NoError(t, os.MkdirAll(skillDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(`---
name: brainstorming
description: "Creative exploration"
---
# Brainstorming`), 0644))

	// Skill dir without SKILL.md should be skipped.
	require.NoError(t, os.MkdirAll(filepath.Join(claudeDir, "skills", "empty-dir"), 0755))

	items, err := ScanGlobal(claudeDir)
	require.NoError(t, err)

	assert.Len(t, items, 3)

	// Verify commands.
	var cmds, skills []Item
	for _, item := range items {
		if item.Type == TypeCommand {
			cmds = append(cmds, item)
		} else {
			skills = append(skills, item)
		}
	}

	assert.Len(t, cmds, 2)
	assert.Len(t, skills, 1)

	// Check review-pr command.
	var reviewPR *Item
	for i := range cmds {
		if cmds[i].Name == "review-pr" {
			reviewPR = &cmds[i]
		}
	}
	require.NotNil(t, reviewPR)
	assert.Equal(t, "Review a pull request", reviewPR.Description)
	assert.Equal(t, SourceGlobal, reviewPR.Source)
	assert.Equal(t, "global", reviewPR.SourceLabel)
	assert.Equal(t, "cmd:global:review-pr", reviewPR.Key())
	assert.Contains(t, reviewPR.Content, "# Review PR")

	// Check deploy command (no frontmatter).
	var deploy *Item
	for i := range cmds {
		if cmds[i].Name == "deploy" {
			deploy = &cmds[i]
		}
	}
	require.NotNil(t, deploy)
	assert.Empty(t, deploy.Description)

	// Check skill.
	assert.Equal(t, "brainstorming", skills[0].Name)
	assert.Equal(t, "Creative exploration", skills[0].Description)
	assert.Equal(t, "skill:global:brainstorming", skills[0].Key())
}

func TestScanGlobal_MissingDirs(t *testing.T) {
	claudeDir := t.TempDir()
	// No commands/ or skills/ directories exist.
	items, err := ScanGlobal(claudeDir)
	require.NoError(t, err)
	assert.Empty(t, items)
}

// ─── ScanProject ───────────────────────────────────────────────────────────

func TestScanProject(t *testing.T) {
	projectDir := t.TempDir()

	// Create project .claude/commands/ structure.
	commandsDir := filepath.Join(projectDir, ".claude", "commands")
	require.NoError(t, os.MkdirAll(commandsDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(commandsDir, "test.md"), []byte(`---
description: Run project tests
---
# Test`), 0644))

	// Create project .claude/skills/ structure.
	skillDir := filepath.Join(projectDir, ".claude", "skills", "onboarding")
	require.NoError(t, os.MkdirAll(skillDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(`---
description: Onboarding guide
---
# Onboarding`), 0644))

	items, err := ScanProject(projectDir)
	require.NoError(t, err)
	assert.Len(t, items, 2)

	// Verify source label uses project basename.
	for _, item := range items {
		assert.Equal(t, SourceProject, item.Source)
		assert.Equal(t, filepath.Base(projectDir), item.SourceLabel)
	}

	// Check key format.
	var cmd *Item
	for i := range items {
		if items[i].Type == TypeCommand {
			cmd = &items[i]
		}
	}
	require.NotNil(t, cmd)
	assert.Equal(t, "cmd:project:"+filepath.Base(projectDir)+":test", cmd.Key())
}

func TestScanProject_MissingClaudeDir(t *testing.T) {
	projectDir := t.TempDir()
	items, err := ScanProject(projectDir)
	require.NoError(t, err)
	assert.Empty(t, items)
}

// ─── ScanPlugins ───────────────────────────────────────────────────────────

func TestScanPlugins(t *testing.T) {
	claudeDir := t.TempDir()

	// Create installed_plugins.json.
	pluginsDir := filepath.Join(claudeDir, "plugins")
	require.NoError(t, os.MkdirAll(pluginsDir, 0755))

	// Create cache directories for two plugins.
	commitCmdDir := filepath.Join(claudeDir, "plugins", "cache", "my-marketplace", "commit-commands", "1.0.0")
	require.NoError(t, os.MkdirAll(filepath.Join(commitCmdDir, "commands"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(commitCmdDir, "commands", "commit.md"), []byte(`---
description: Create a git commit
---
# Commit`), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(commitCmdDir, "commands", "push.md"), []byte(`# Push`), 0644))

	superDir := filepath.Join(claudeDir, "plugins", "cache", "my-marketplace", "superpowers", "2.0.0")
	require.NoError(t, os.MkdirAll(filepath.Join(superDir, "skills", "tdd"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(superDir, "skills", "tdd", "SKILL.md"), []byte(`---
name: tdd
description: "Test-driven development"
---
# TDD`), 0644))
	require.NoError(t, os.MkdirAll(filepath.Join(superDir, "commands"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(superDir, "commands", "plan.md"), []byte(`# Plan`), 0644))

	ip := installedPluginsJSON{
		Version: 2,
		Plugins: map[string][]pluginInstallJSON{
			"commit-commands@my-marketplace": {
				{InstallPath: commitCmdDir, Version: "1.0.0"},
			},
			"superpowers@my-marketplace": {
				{InstallPath: superDir, Version: "2.0.0"},
			},
		},
	}
	data, err := json.MarshalIndent(ip, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(pluginsDir, "installed_plugins.json"), data, 0644))

	items, err := ScanPlugins(claudeDir)
	require.NoError(t, err)

	// 2 commands from commit-commands, 1 command + 1 skill from superpowers = 4.
	assert.Len(t, items, 4)

	// Verify all items are from plugins.
	for _, item := range items {
		assert.Equal(t, SourcePlugin, item.Source)
	}

	// Find the commit command specifically.
	var commitCmd *Item
	for i := range items {
		if items[i].Name == "commit" && items[i].SourceLabel == "commit-commands" {
			commitCmd = &items[i]
		}
	}
	require.NotNil(t, commitCmd)
	assert.Equal(t, "Create a git commit", commitCmd.Description)
	assert.Equal(t, "cmd:plugin:commit-commands:commit", commitCmd.Key())

	// Find the TDD skill.
	var tddSkill *Item
	for i := range items {
		if items[i].Name == "tdd" {
			tddSkill = &items[i]
		}
	}
	require.NotNil(t, tddSkill)
	assert.Equal(t, TypeSkill, tddSkill.Type)
	assert.Equal(t, "Test-driven development", tddSkill.Description)
	assert.Equal(t, "skill:plugin:superpowers:tdd", tddSkill.Key())
}

func TestScanPlugins_NoFile(t *testing.T) {
	claudeDir := t.TempDir()
	items, err := ScanPlugins(claudeDir)
	require.NoError(t, err)
	assert.Empty(t, items)
}

func TestScanPlugins_EmptyPlugins(t *testing.T) {
	claudeDir := t.TempDir()
	pluginsDir := filepath.Join(claudeDir, "plugins")
	require.NoError(t, os.MkdirAll(pluginsDir, 0755))

	ip := installedPluginsJSON{Version: 2, Plugins: map[string][]pluginInstallJSON{}}
	data, err := json.Marshal(ip)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(pluginsDir, "installed_plugins.json"), data, 0644))

	items, err := ScanPlugins(claudeDir)
	require.NoError(t, err)
	assert.Empty(t, items)
}

func TestScanPlugins_MissingInstallPath(t *testing.T) {
	claudeDir := t.TempDir()
	pluginsDir := filepath.Join(claudeDir, "plugins")
	require.NoError(t, os.MkdirAll(pluginsDir, 0755))

	// Point to a nonexistent install path — should skip gracefully.
	ip := installedPluginsJSON{
		Version: 2,
		Plugins: map[string][]pluginInstallJSON{
			"ghost@marketplace": {
				{InstallPath: "/nonexistent/path", Version: "1.0.0"},
			},
		},
	}
	data, err := json.Marshal(ip)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(pluginsDir, "installed_plugins.json"), data, 0644))

	items, err := ScanPlugins(claudeDir)
	require.NoError(t, err)
	assert.Empty(t, items)
}

// ─── ScanAll ───────────────────────────────────────────────────────────────

func TestScanAll(t *testing.T) {
	claudeDir := t.TempDir()
	projectDir := t.TempDir()

	// Set up global command.
	commandsDir := filepath.Join(claudeDir, "commands")
	require.NoError(t, os.MkdirAll(commandsDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(commandsDir, "global-cmd.md"), []byte("# Global"), 0644))

	// Set up project command.
	projCmdDir := filepath.Join(projectDir, ".claude", "commands")
	require.NoError(t, os.MkdirAll(projCmdDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(projCmdDir, "project-cmd.md"), []byte("# Project"), 0644))

	// No installed_plugins.json — plugins scan returns empty.
	result, err := ScanAll(claudeDir, []string{projectDir})
	require.NoError(t, err)
	assert.Len(t, result.Items, 2)

	// Verify sources.
	sources := map[Source]int{}
	for _, item := range result.Items {
		sources[item.Source]++
	}
	assert.Equal(t, 1, sources[SourceGlobal])
	assert.Equal(t, 1, sources[SourceProject])
}

func TestScanAll_NilProjectDirs(t *testing.T) {
	claudeDir := t.TempDir()

	result, err := ScanAll(claudeDir, nil)
	require.NoError(t, err)
	assert.Empty(t, result.Items)
}
