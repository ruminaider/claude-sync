package tui

import (
	"fmt"

	"github.com/ruminaider/claude-sync/internal/commands"
)

// recommendation represents a context-aware suggestion for the user.
type recommendation struct {
	icon   string // "⚠" for warning, "✗" for error, "💡" for suggestion
	title  string // e.g., "Config is 3 commits behind"
	detail string // e.g., "Your team pushed updates 4h ago"
	action actionItem
}

// actionItem represents an executable action (used by both recommendations and intents).
type actionItem struct {
	id     string   // e.g., "pull", "approve", "plugin-update"
	label  string   // e.g., "Pull and apply now"
	inline bool     // true = execute inline with spinner, false = navigate to sub-view
	args   []string // optional arguments (e.g., plugin key for update)
}

// buildRecommendations generates prioritized recommendations based on current state.
func buildRecommendations(state commands.MenuState) []recommendation {
	recs := []recommendation{}

	if !state.ConfigExists {
		return recs
	}

	// 1. Merge conflicts (highest priority)
	if state.HasConflicts {
		recs = append(recs, recommendation{
			icon:   "✗",
			title:  "Merge conflicts need resolution",
			detail: "Config has conflicts that must be resolved before syncing",
			action: actionItem{
				id:     ActionConflicts,
				label:  "Resolve conflicts",
				inline: true,
			},
		})
	}

	// 2. Config behind remote
	if state.CommitsBehind > 0 {
		recs = append(recs, recommendation{
			icon:  "⚠",
			title: fmt.Sprintf("Config is %d commits behind", state.CommitsBehind),
			action: actionItem{
				id:     ActionPull,
				label:  "Pull and apply now",
				inline: true,
			},
		})
	}

	// 3. Pending approval changes
	if state.HasPending {
		recs = append(recs, recommendation{
			icon:   "⚠",
			title:  "Pending changes awaiting your review",
			detail: "High-risk changes require explicit approval",
			action: actionItem{
				id:    ActionApprove,
				label: "Review and decide",
			},
		})
	}

	// 4. Plugin updates (one per plugin)
	for _, p := range state.Plugins {
		if p.LatestVersion != "" && p.LatestVersion != p.PinVersion {
			recs = append(recs, recommendation{
				icon:   "💡",
				title:  fmt.Sprintf("%s has an update available", p.Name),
				detail: fmt.Sprintf("Pinned at v%s, latest is v%s", p.PinVersion, p.LatestVersion),
				action: actionItem{
					id:     ActionPluginUpdate,
					label:  fmt.Sprintf("Update to v%s", p.LatestVersion),
					inline: true,
					args:   []string{p.Key},
				},
			})
		}
	}

	// 5. No profiles exist
	if len(state.Profiles) == 0 {
		recs = append(recs, recommendation{
			icon:   "💡",
			title:  "Create your first settings profile",
			detail: "Profiles let you switch between work/personal configurations",
			action: actionItem{
				id:    "create-profile",
				label: "Create a profile",
			},
		})
	}

	// 6. No plugins installed
	if len(state.Plugins) == 0 {
		recs = append(recs, recommendation{
			icon:   "💡",
			title:  "Add your first plugin",
			detail: "Plugins extend Claude Code with new capabilities",
			action: actionItem{
				id:    ActionBrowsePlugins,
				label: "Browse available plugins",
			},
		})
	}

	// 7. State detection warnings
	for _, w := range state.Warnings {
		recs = append(recs, recommendation{
			icon:  "⚠",
			title: w,
			action: actionItem{
				id:    "warning",
				label: "Dismiss",
			},
		})
	}

	return recs
}
