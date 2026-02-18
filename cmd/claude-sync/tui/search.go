package tui

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
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
				"-t", "f",
				"-d", "4",
				"CLAUDE.md",
				"--search-path", home,
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

		// Filter to only .claude/CLAUDE.md paths.
		var filtered []string
		for _, p := range rawPaths {
			p = strings.TrimSpace(p)
			if p == "" {
				continue
			}
			dir := filepath.Dir(p)
			if filepath.Base(dir) == ".claude" && filepath.Base(p) == "CLAUDE.md" {
				filtered = append(filtered, p)
			}
		}

		return SearchDoneMsg{Paths: filtered}
	}
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
