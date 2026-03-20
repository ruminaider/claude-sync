package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/ruminaider/claude-sync/internal/commands"
)

// intent represents a goal-oriented action the user can take.
type intent struct {
	label  string // "Add or discover new plugins"
	hint   string // shown on right: "work →" or "enter"
	action actionItem
}

// buildIntents returns the static list of intent-based actions.
// The profile label is contextual based on whether a profile is active.
func buildIntents(state commands.MenuState) []intent {
	profileLabel := "Activate a settings profile"
	profileHint := "\u2192"
	if state.ActiveProfile != "" {
		profileLabel = "Switch settings profile"
		profileHint = state.ActiveProfile + " \u2192"
	}

	return []intent{
		{
			label: "View active plugins",
			hint:  "\u2192",
			action: actionItem{
				id:    "view-plugins",
				label: "View active plugins",
			},
		},
		{
			label: "Join a shared config",
			hint:  "\u2192",
			action: actionItem{
				id:    "join-config",
				label: "Join a shared config",
			},
		},
		{
			label: "Add or discover new plugins",
			hint:  "\u2192",
			action: actionItem{
				id:    "browse-plugins",
				label: "Add or discover new plugins",
			},
		},
		{
			label: profileLabel,
			hint:  profileHint,
			action: actionItem{
				id:    "switch-profile",
				label: profileLabel,
			},
		},
		{
			label: "Push my local changes",
			hint:  "enter",
			action: actionItem{
				id:     "push-changes",
				label:  "Push my local changes",
				inline: true,
			},
		},
		{
			label: "Edit my full config",
			hint:  "\u2192",
			action: actionItem{
				id:    "edit-config",
				label: "Edit my full config",
			},
		},
		{
			label: "Import MCP servers",
			hint:  "enter",
			action: actionItem{
				id:     "import-mcp",
				label:  "Import MCP servers",
				inline: true,
			},
		},
		{
			label: "View full config details",
			hint:  "\u2192",
			action: actionItem{
				id:    "view-config",
				label: "View full config details",
			},
		},
	}
}

// actionItemCount returns the total number of selectable items across both sections.
func actionItemCount(recs []recommendation, intents []intent) int {
	return len(recs) + len(intents)
}

// selectedAction returns the actionItem at the given cursor position.
func selectedAction(recs []recommendation, intents []intent, cursor int) *actionItem {
	if cursor < len(recs) {
		return &recs[cursor].action
	}
	intentIdx := cursor - len(recs)
	if intentIdx < len(intents) {
		return &intents[intentIdx].action
	}
	return nil
}

// renderRecsSectionWithState renders the "Needs attention" / "Status" section with execution state.
func renderRecsSectionWithState(recs []recommendation, cursor int, innerWidth int,
	executing bool, executingActionID string, results map[string]actionResultMsg) string {
	var lines []string

	if len(recs) == 0 {
		lines = append(lines, sectionHeader("Status"))
		lines = append(lines, stGreen.Render("\u2713 Everything looks good"))
		return strings.Join(lines, "\n")
	}

	lines = append(lines, sectionHeader("Needs attention"))

	for i, rec := range recs {
		// Recommendation title with icon
		lines = append(lines, stText.Render(fmt.Sprintf("%s %s", rec.icon, rec.title)))

		// Optional detail line
		if rec.detail != "" {
			lines = append(lines, stDim.Render("  "+rec.detail))
		}

		// Check for execution state on this item
		if result, hasResult := results[rec.action.id]; hasResult {
			// Show result
			if result.success {
				lines = append(lines, stGreen.Render("\u2713 "+result.message))
			} else {
				errMsg := result.message
				if errMsg == "" && result.err != nil {
					errMsg = result.err.Error()
				}
				lines = append(lines, stRed.Render("\u2717 "+errMsg))
			}
		} else if executing && executingActionID == rec.action.id {
			// Show executing spinner
			lines = append(lines, stYellow.Render("\u27f3 "+rec.action.label+"..."))
		} else {
			// Normal action line (selectable)
			hint := "enter"
			if !rec.action.inline {
				hint = "\u2192"
			}

			if cursor == i {
				actionLine := stBlue.Render("> " + rec.action.label)
				hintText := stDim.Render(hint)
				gap := innerWidth - lipgloss.Width(actionLine) - lipgloss.Width(hintText)
				if gap < 1 {
					gap = 1
				}
				lines = append(lines, actionLine+strings.Repeat(" ", gap)+hintText)
			} else {
				actionLine := stDim.Render("  " + rec.action.label)
				lines = append(lines, actionLine)
			}
		}

		// Add spacing between recommendations (but not after the last one)
		if i < len(recs)-1 {
			lines = append(lines, "")
		}
	}

	return strings.Join(lines, "\n")
}

// renderIntentsSectionWithState renders the "I want to..." section with execution state.
func renderIntentsSectionWithState(intents []intent, recCount, cursor int, innerWidth int,
	executing bool, executingActionID string, results map[string]actionResultMsg) string {
	var lines []string
	lines = append(lines, sectionHeader("I want to..."))

	for i, it := range intents {
		globalIdx := recCount + i
		isSelected := cursor == globalIdx

		// Check for execution state on this item
		if result, hasResult := results[it.action.id]; hasResult {
			// Show result
			if result.success {
				lines = append(lines, stGreen.Render("\u2713 "+result.message))
			} else {
				errMsg := result.message
				if errMsg == "" && result.err != nil {
					errMsg = result.err.Error()
				}
				lines = append(lines, stRed.Render("\u2717 "+errMsg))
			}
			continue
		}

		if executing && executingActionID == it.action.id {
			// Show executing spinner
			lines = append(lines, stYellow.Render("\u27f3 "+it.action.label+"..."))
			continue
		}

		if isSelected {
			label := stBlue.Render("> " + it.label)
			hintText := stDim.Render(it.hint)
			gap := innerWidth - lipgloss.Width(label) - lipgloss.Width(hintText)
			if gap < 1 {
				gap = 1
			}
			lines = append(lines, label+strings.Repeat(" ", gap)+hintText)
		} else {
			label := stText.Render("  " + it.label)
			hintText := stDim.Render(it.hint)
			gap := innerWidth - lipgloss.Width(label) - lipgloss.Width(hintText)
			if gap < 1 {
				gap = 1
			}
			lines = append(lines, label+strings.Repeat(" ", gap)+hintText)
		}
	}

	return strings.Join(lines, "\n")
}

// Backward-compatible rendering functions used by tests.

// renderActions renders the combined action screen with recommendations and intents.
func renderActions(recs []recommendation, intents []intent, cursor int, width, height int) string {
	return renderActionsWithState(recs, intents, cursor, width, height, false, "", nil)
}

// renderActionsWithState renders the combined action screen with execution state.
// This is kept for backward compatibility with tests.
func renderActionsWithState(recs []recommendation, intents []intent, cursor int, width, height int,
	executing bool, executingActionID string, results map[string]actionResultMsg) string {
	maxWidth, innerWidth := clampWidth(width)

	var sections []string

	// --- Needs attention section ---
	sections = append(sections, renderRecsSectionWithState(recs, cursor, innerWidth, executing, executingActionID, results))

	// --- I want to... section ---
	sections = append(sections, renderIntentsSectionWithState(intents, len(recs), cursor, innerWidth, executing, executingActionID, results))

	// --- Footer ---
	sections = append(sections, renderMainFooter())

	content := strings.Join(sections, "\n\n")

	boxStyle := contentBox(maxWidth, colorSurface1)

	return boxStyle.Render(content)
}

// filterRecommendations filters recommendations by case-insensitive substring match
// on title, detail, or action label.
func filterRecommendations(recs []recommendation, query string) []recommendation {
	if query == "" {
		return recs
	}
	query = strings.ToLower(query)
	var filtered []recommendation
	for _, r := range recs {
		if strings.Contains(strings.ToLower(r.title), query) ||
			strings.Contains(strings.ToLower(r.detail), query) ||
			strings.Contains(strings.ToLower(r.action.label), query) {
			filtered = append(filtered, r)
		}
	}
	return filtered
}

// filterIntents filters intents by case-insensitive substring match on label.
func filterIntents(intents []intent, query string) []intent {
	if query == "" {
		return intents
	}
	query = strings.ToLower(query)
	var filtered []intent
	for _, i := range intents {
		if strings.Contains(strings.ToLower(i.label), query) {
			filtered = append(filtered, i)
		}
	}
	return filtered
}
