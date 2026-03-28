package tui

import (
	"fmt"
	"strings"

	"github.com/ruminaider/claude-sync/internal/commands"
)

// extractRepoName parses an owner/repo name from a git remote URL.
// Handles HTTPS (https://github.com/owner/repo.git) and SSH (git@github.com:owner/repo.git).
// Returns "claude-sync" as a fallback if parsing fails.
func extractRepoName(remoteURL string) string {
	url := strings.TrimSpace(remoteURL)
	if url == "" {
		return "claude-sync"
	}

	// Strip trailing .git
	url = strings.TrimSuffix(url, ".git")

	// SSH format: git@github.com:owner/repo
	if strings.HasPrefix(url, "git@") {
		// git@github.com:owner/repo -> owner/repo
		if idx := strings.Index(url, ":"); idx >= 0 {
			path := url[idx+1:]
			if path != "" {
				return path
			}
		}
		return "claude-sync"
	}

	// HTTPS format: https://github.com/owner/repo
	// Find the last two path segments (owner/repo)
	if idx := strings.Index(url, "://"); idx >= 0 {
		path := url[idx+3:]
		// Remove host
		if slashIdx := strings.Index(path, "/"); slashIdx >= 0 {
			path = path[slashIdx+1:]
			if path != "" {
				return path
			}
		}
	}

	return "claude-sync"
}

// buildBanner renders a 2-3 line status banner for the menu header.
//
// Line 1: repoName (role)
// Line 2: sync indicator · plugin count · profile
// Line 3 (conditional): pending/conflict warning
func buildBanner(state commands.MenuState) string {
	var lines []string

	// Line 1: repo name and role
	repoName := extractRepoName(state.RemoteURL)
	line1 := stBlue.Render(repoName)
	if state.Role != "" {
		line1 += stDim.Render(" (" + state.Role + ")")
	}
	lines = append(lines, line1)

	// Line 2: sync indicator · plugin count · profile
	var parts []string

	// Sync indicator
	switch {
	case state.CommitsBehind > 0 && state.CommitsAhead > 0:
		parts = append(parts, stYellow.Render(fmt.Sprintf("↓ %d behind", state.CommitsBehind)))
		parts = append(parts, stYellow.Render(fmt.Sprintf("↑ %d local", state.CommitsAhead)))
	case state.CommitsBehind > 0:
		parts = append(parts, stYellow.Render(fmt.Sprintf("↓ %d behind", state.CommitsBehind)))
	case state.CommitsAhead > 0:
		parts = append(parts, stYellow.Render(fmt.Sprintf("↑ %d local", state.CommitsAhead)))
	default:
		// Both 0 or both -1 (unknown)
		parts = append(parts, stGreen.Render("✓ synced"))
	}

	// Plugin count
	if state.PluginCount > 0 {
		parts = append(parts, stDim.Render(fmt.Sprintf("%d plugins", state.PluginCount)))
	}

	// Profile
	profileName := state.ActiveProfile
	if profileName == "" {
		profileName = "none"
	}
	parts = append(parts, stDim.Render("profile: "+profileName))

	lines = append(lines, strings.Join(parts, stDim.Render(" · ")))

	// Line 3 (conditional): pending changes or conflicts
	if state.HasConflicts {
		warning := stYellow.Render("⚠ conflicts need review")
		hint := stDim.Render(" ⏎ review")
		lines = append(lines, warning+hint)
	} else if state.HasPending {
		warning := stYellow.Render(fmt.Sprintf("⚠ %d pending changes need review", state.PendingCount))
		hint := stDim.Render(" ⏎ review")
		lines = append(lines, warning+hint)
	}

	return strings.Join(lines, "\n")
}
