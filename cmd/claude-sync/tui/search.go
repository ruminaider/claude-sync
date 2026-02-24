package tui

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ruminaider/claude-sync/internal/claudecode"
	"github.com/ruminaider/claude-sync/internal/cmdskill"
)

// SearchResultMsg carries a single discovered CLAUDE.md file path.
type SearchResultMsg struct{ Path string }

// SearchDoneMsg is sent when the background search is complete. It contains
// all discovered CLAUDE.md file paths.
type SearchDoneMsg struct{ Paths []string }

// SearchClaudeMD returns a tea.Cmd that searches for CLAUDE.md files under the
// home directory. It first tries the fd command for speed and falls back to
// find if fd is unavailable. Only paths matching */.claude/CLAUDE.md are
// returned.
func SearchClaudeMD() tea.Cmd {
	return func() tea.Msg {
		home, err := os.UserHomeDir()
		if err != nil {
			return SearchDoneMsg{}
		}

		var rawPaths []string

		// Try fd first.
		if fdPath, err := exec.LookPath("fd"); err == nil {
			out, err := exec.Command(
				fdPath,
				"-I",
				"-t", "f",
				"-d", "4",
				"CLAUDE.md",
				home,
				"-E", "node_modules",
				"-E", ".git",
				"-E", "Library",
				"-E", ".cache",
				"-E", ".Trash",
				"-E", "go/pkg",
			).Output()
			if err == nil {
				rawPaths = splitLines(string(out))
			}
		}

		// Fallback to find.
		if len(rawPaths) == 0 {
			out, _ := exec.Command(
				"find", home,
				"-maxdepth", "4",
				"-name", "CLAUDE.md",
				"-not", "-path", "*/node_modules/*",
				"-not", "-path", "*/.git/*",
				"-not", "-path", "*/Library/*",
				"-not", "-path", "*/.cache/*",
				"-not", "-path", "*/.Trash/*",
				"-not", "-path", "*/go/pkg/*",
			).Output()
			rawPaths = splitLines(string(out))
		}

		// Keep all CLAUDE.md files except the global ~/.claude/CLAUDE.md
		// (already loaded by the initial scan).
		globalClaude := filepath.Join(home, ".claude", "CLAUDE.md")
		var filtered []string
		for _, p := range rawPaths {
			p = strings.TrimSpace(p)
			if p == "" || filepath.Base(p) != "CLAUDE.md" {
				continue
			}
			absP, _ := filepath.Abs(p)
			absGlobal, _ := filepath.Abs(globalClaude)
			if absP == absGlobal {
				continue
			}
			filtered = append(filtered, p)
		}

		return SearchDoneMsg{Paths: filtered}
	}
}

// MCPSearchDoneMsg is sent when the background MCP search is complete.
type MCPSearchDoneMsg struct {
	Servers map[string]json.RawMessage // server name → config
	Sources map[string]string          // server name → shortened source path
}

// SearchMCPConfigs returns a tea.Cmd that searches for .mcp.json files under
// the home directory. It uses the same fd/find pattern as SearchClaudeMD.
// Discovered servers are deduplicated by name (first-found wins), and the
// global ~/.claude/.mcp.json is excluded (already scanned by InitScan).
func SearchMCPConfigs() tea.Cmd {
	return func() tea.Msg {
		home, err := os.UserHomeDir()
		if err != nil {
			return MCPSearchDoneMsg{}
		}

		globalMCP := filepath.Join(home, ".claude", ".mcp.json")
		var rawPaths []string

		// Try fd first. -H is needed because .mcp.json is a hidden file.
		if fdPath, err := exec.LookPath("fd"); err == nil {
			out, err := exec.Command(
				fdPath,
				"-H", "-I",
				"-t", "f",
				"-d", "4",
				".mcp.json",
				home,
				"-E", "node_modules",
				"-E", ".git",
				"-E", "Library",
				"-E", ".cache",
				"-E", ".Trash",
				"-E", "go/pkg",
			).Output()
			if err == nil {
				rawPaths = splitLines(string(out))
			}
		}

		// Fallback to find.
		if len(rawPaths) == 0 {
			out, _ := exec.Command(
				"find", home,
				"-maxdepth", "4",
				"-name", ".mcp.json",
				"-not", "-path", "*/node_modules/*",
				"-not", "-path", "*/.git/*",
				"-not", "-path", "*/Library/*",
				"-not", "-path", "*/.cache/*",
				"-not", "-path", "*/.Trash/*",
				"-not", "-path", "*/go/pkg/*",
			).Output()
			rawPaths = splitLines(string(out))
		}

		// Filter: keep project-root .mcp.json and project .claude/.mcp.json,
		// but skip the global ~/.claude/.mcp.json.
		servers := make(map[string]json.RawMessage)
		sources := make(map[string]string)

		for _, p := range rawPaths {
			p = strings.TrimSpace(p)
			if p == "" {
				continue
			}
			// Skip global MCP config.
			absP, _ := filepath.Abs(p)
			absGlobal, _ := filepath.Abs(globalMCP)
			if absP == absGlobal {
				continue
			}

			mcpMap, err := claudecode.ReadMCPConfigFile(p)
			if err != nil || len(mcpMap) == 0 {
				continue
			}

			sourcePath := shortenPath(filepath.Dir(p))
			for name, cfg := range mcpMap {
				if _, exists := servers[name]; !exists {
					servers[name] = cfg
					sources[name] = sourcePath
				}
			}
		}

		return MCPSearchDoneMsg{Servers: servers, Sources: sources}
	}
}

// shortenPath replaces the home directory prefix with ~ for display.
func shortenPath(path string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}
	if strings.HasPrefix(path, home) {
		return "~" + path[len(home):]
	}
	return path
}

// splitLines splits output by newlines, filtering empty strings.
func splitLines(s string) []string {
	raw := strings.Split(s, "\n")
	var result []string
	for _, line := range raw {
		line = strings.TrimSpace(line)
		if line != "" {
			result = append(result, line)
		}
	}
	return result
}

// CmdSkillSearchDoneMsg is sent when the background commands/skills search is
// complete. It contains all discovered project-local command and skill items.
type CmdSkillSearchDoneMsg struct {
	Items []cmdskill.Item
}

// SearchCommandsSkills returns a tea.Cmd that searches for project directories
// containing .claude/commands/ or .claude/skills/ under the home directory.
// It uses fd/find to locate .claude directories, then calls cmdskill.ScanProject
// for each found project.
func SearchCommandsSkills() tea.Cmd {
	return func() tea.Msg {
		home, err := os.UserHomeDir()
		if err != nil {
			return CmdSkillSearchDoneMsg{}
		}

		globalClaude := filepath.Join(home, ".claude")
		var rawPaths []string

		// Try fd first. Look for directories named ".claude" that contain
		// commands/ or skills/ subdirectories.
		if fdPath, err := exec.LookPath("fd"); err == nil {
			out, err := exec.Command(
				fdPath,
				"-H", "-I",
				"-t", "d",
				"-d", "4",
				"^.claude$",
				home,
				"-E", "node_modules",
				"-E", ".git",
				"-E", "Library",
				"-E", ".cache",
				"-E", ".Trash",
				"-E", "go/pkg",
			).Output()
			if err == nil {
				rawPaths = splitLines(string(out))
			}
		}

		// Fallback to find.
		if len(rawPaths) == 0 {
			out, _ := exec.Command(
				"find", home,
				"-maxdepth", "4",
				"-type", "d",
				"-name", ".claude",
				"-not", "-path", "*/node_modules/*",
				"-not", "-path", "*/.git/*",
				"-not", "-path", "*/Library/*",
				"-not", "-path", "*/.cache/*",
				"-not", "-path", "*/.Trash/*",
				"-not", "-path", "*/go/pkg/*",
			).Output()
			rawPaths = splitLines(string(out))
		}

		var allItems []cmdskill.Item

		for _, p := range rawPaths {
			p = strings.TrimSpace(p)
			if p == "" {
				continue
			}

			// Skip the global ~/.claude/ directory.
			absP, _ := filepath.Abs(p)
			absGlobal, _ := filepath.Abs(globalClaude)
			if absP == absGlobal {
				continue
			}

			// The project root is the parent of .claude/.
			projectDir := filepath.Dir(p)

			items, err := cmdskill.ScanProject(projectDir)
			if err != nil || len(items) == 0 {
				continue
			}

			allItems = append(allItems, items...)
		}

		return CmdSkillSearchDoneMsg{Items: allItems}
	}
}
