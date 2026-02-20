package cmdskill

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ItemType distinguishes commands from skills.
type ItemType int

const (
	TypeCommand ItemType = iota
	TypeSkill
)

// Source identifies where an item was discovered.
type Source int

const (
	SourcePlugin Source = iota
	SourceGlobal
	SourceProject
)

// Item represents a single command (.md file) or skill (SKILL.md directory).
type Item struct {
	Name        string   // filename without extension for commands, dir name for skills
	Type        ItemType
	Source      Source
	SourceLabel string // plugin name, "global", or project path
	FilePath    string // absolute path to .md or SKILL.md
	Description string // from YAML frontmatter (if any)
	Content     string // full file content for preview
}

// Key returns a unique identifier for the item.
//
// Format:
//   - cmd:global:review-pr
//   - skill:global:brainstorming
//   - cmd:plugin:commit-commands:commit
//   - cmd:project:myproject:deploy
func (i Item) Key() string {
	prefix := "cmd"
	if i.Type == TypeSkill {
		prefix = "skill"
	}

	switch i.Source {
	case SourcePlugin:
		return fmt.Sprintf("%s:plugin:%s:%s", prefix, i.SourceLabel, i.Name)
	case SourceGlobal:
		return fmt.Sprintf("%s:global:%s", prefix, i.Name)
	case SourceProject:
		return fmt.Sprintf("%s:project:%s:%s", prefix, i.SourceLabel, i.Name)
	default:
		return fmt.Sprintf("%s:unknown:%s", prefix, i.Name)
	}
}

// ScanResult holds the combined results from all sources.
type ScanResult struct {
	Items []Item
}

// ScanAll scans all sources and returns combined results.
// claudeDir is typically ~/.claude/.
// projectDirs are additional project directories to scan (can be nil).
func ScanAll(claudeDir string, projectDirs []string) (*ScanResult, error) {
	var all []Item

	plugins, err := ScanPlugins(claudeDir)
	if err != nil {
		return nil, fmt.Errorf("scanning plugins: %w", err)
	}
	all = append(all, plugins...)

	global, err := ScanGlobal(claudeDir)
	if err != nil {
		return nil, fmt.Errorf("scanning global: %w", err)
	}
	all = append(all, global...)

	for _, dir := range projectDirs {
		project, err := ScanProject(dir)
		if err != nil {
			return nil, fmt.Errorf("scanning project %s: %w", dir, err)
		}
		all = append(all, project...)
	}

	return &ScanResult{Items: all}, nil
}

// installedPluginsJSON mirrors the structure of installed_plugins.json.
type installedPluginsJSON struct {
	Version int                            `json:"version"`
	Plugins map[string][]pluginInstallJSON `json:"plugins"`
}

// pluginInstallJSON represents a single installation entry.
type pluginInstallJSON struct {
	InstallPath string `json:"installPath"`
	Version     string `json:"version"`
}

// ScanPlugins walks the plugin cache to find commands and skills from installed plugins.
// It uses installed_plugins.json to determine which plugins/versions are active.
func ScanPlugins(claudeDir string) ([]Item, error) {
	ipPath := filepath.Join(claudeDir, "plugins", "installed_plugins.json")
	data, err := os.ReadFile(ipPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading installed plugins: %w", err)
	}

	var ip installedPluginsJSON
	if err := json.Unmarshal(data, &ip); err != nil {
		return nil, fmt.Errorf("parsing installed plugins: %w", err)
	}

	var items []Item

	for key, installs := range ip.Plugins {
		if len(installs) == 0 {
			continue
		}
		// Use the first installation entry (primary).
		install := installs[0]
		installPath := install.InstallPath
		if installPath == "" {
			continue
		}

		// Plugin name is the part before '@' in the key (e.g., "commit-commands@claude-plugins-official").
		pluginName := key
		if idx := strings.Index(key, "@"); idx >= 0 {
			pluginName = key[:idx]
		}

		// Scan commands/ directory.
		cmds, err := scanCommandsDir(installPath, SourcePlugin, pluginName)
		if err != nil {
			continue // skip on error
		}
		items = append(items, cmds...)

		// Scan skills/ directory.
		skills, err := scanSkillsDir(installPath, SourcePlugin, pluginName)
		if err != nil {
			continue
		}
		items = append(items, skills...)
	}

	return items, nil
}

// ScanGlobal walks claudeDir/commands/ and claudeDir/skills/ for user-created items.
func ScanGlobal(claudeDir string) ([]Item, error) {
	var items []Item

	cmds, err := scanCommandsDir(claudeDir, SourceGlobal, "global")
	if err != nil {
		return nil, err
	}
	items = append(items, cmds...)

	skills, err := scanSkillsDir(claudeDir, SourceGlobal, "global")
	if err != nil {
		return nil, err
	}
	items = append(items, skills...)

	return items, nil
}

// ScanProject walks dir/.claude/commands/ and dir/.claude/skills/ for project-local items.
func ScanProject(projectDir string) ([]Item, error) {
	claudeDir := filepath.Join(projectDir, ".claude")
	label := filepath.Base(projectDir)

	var items []Item

	cmds, err := scanCommandsDir(claudeDir, SourceProject, label)
	if err != nil {
		return nil, err
	}
	items = append(items, cmds...)

	skills, err := scanSkillsDir(claudeDir, SourceProject, label)
	if err != nil {
		return nil, err
	}
	items = append(items, skills...)

	return items, nil
}

// scanCommandsDir reads .md files from dir/commands/.
func scanCommandsDir(dir string, source Source, label string) ([]Item, error) {
	commandsDir := filepath.Join(dir, "commands")
	entries, err := os.ReadDir(commandsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading commands dir %s: %w", commandsDir, err)
	}

	var items []Item
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}

		filePath := filepath.Join(commandsDir, entry.Name())
		content, err := os.ReadFile(filePath)
		if err != nil {
			continue // skip unreadable files
		}

		name := strings.TrimSuffix(entry.Name(), ".md")
		_, desc := ParseFrontmatter(string(content))

		items = append(items, Item{
			Name:        name,
			Type:        TypeCommand,
			Source:      source,
			SourceLabel: label,
			FilePath:    filePath,
			Description: desc,
			Content:     string(content),
		})
	}

	return items, nil
}

// scanSkillsDir reads SKILL.md files from dir/skills/*/.
func scanSkillsDir(dir string, source Source, label string) ([]Item, error) {
	skillsDir := filepath.Join(dir, "skills")
	entries, err := os.ReadDir(skillsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading skills dir %s: %w", skillsDir, err)
	}

	var items []Item
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		skillFile := filepath.Join(skillsDir, entry.Name(), "SKILL.md")
		content, err := os.ReadFile(skillFile)
		if err != nil {
			continue // skip directories without SKILL.md
		}

		_, desc := ParseFrontmatter(string(content))

		items = append(items, Item{
			Name:        entry.Name(),
			Type:        TypeSkill,
			Source:      source,
			SourceLabel: label,
			FilePath:    skillFile,
			Description: desc,
			Content:     string(content),
		})
	}

	return items, nil
}

// ParseFrontmatter extracts name and description from YAML frontmatter in markdown content.
// Frontmatter is delimited by --- lines at the start of the file.
// Returns empty strings if no frontmatter is present.
func ParseFrontmatter(content string) (name, desc string) {
	if !strings.HasPrefix(content, "---") {
		return "", ""
	}

	// Find the closing ---
	rest := content[3:]
	// Skip the newline after opening ---
	if idx := strings.IndexByte(rest, '\n'); idx >= 0 {
		rest = rest[idx+1:]
	} else {
		return "", ""
	}

	endIdx := strings.Index(rest, "\n---")
	if endIdx < 0 {
		return "", ""
	}

	frontmatter := rest[:endIdx]

	// Simple line-by-line YAML parsing for name and description fields.
	for _, line := range strings.Split(frontmatter, "\n") {
		line = strings.TrimSpace(line)
		if k, v, ok := parseYAMLLine(line); ok {
			switch k {
			case "name":
				name = v
			case "description":
				desc = v
			}
		}
	}

	return name, desc
}

// parseYAMLLine extracts a key-value pair from a simple YAML line.
// Returns the key, unquoted value, and whether parsing succeeded.
func parseYAMLLine(line string) (key, value string, ok bool) {
	idx := strings.Index(line, ":")
	if idx < 0 {
		return "", "", false
	}

	key = strings.TrimSpace(line[:idx])
	value = strings.TrimSpace(line[idx+1:])

	// Strip surrounding quotes.
	if len(value) >= 2 {
		if (value[0] == '"' && value[len(value)-1] == '"') ||
			(value[0] == '\'' && value[len(value)-1] == '\'') {
			value = value[1 : len(value)-1]
		}
	}

	return key, value, true
}
