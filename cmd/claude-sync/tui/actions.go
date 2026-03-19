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

// renderActions renders the combined action screen with recommendations and intents.
// This is the backward-compatible version without execution state.
func renderActions(recs []recommendation, intents []intent, cursor int, width, height int) string {
	return renderActionsWithState(recs, intents, cursor, width, height, false, -1, nil)
}

// renderActionsWithState renders the combined action screen with execution state.
func renderActionsWithState(recs []recommendation, intents []intent, cursor int, width, height int,
	executing bool, executingIndex int, results map[int]actionResultMsg) string {
	maxWidth := width - 2
	if maxWidth > 70 {
		maxWidth = 70
	}
	if maxWidth < 30 {
		maxWidth = 30
	}

	// Available width inside the box (subtract border + padding: 2 border + 4 padding)
	innerWidth := maxWidth - 6
	if innerWidth < 20 {
		innerWidth = 20
	}

	var sections []string

	// --- Needs attention section ---
	sections = append(sections, renderRecsSectionWithState(recs, cursor, innerWidth, executing, executingIndex, results))

	// --- Divider ---
	divStyle := lipgloss.NewStyle().Foreground(colorSurface1)
	sections = append(sections, divStyle.Render(strings.Repeat("\u2500", innerWidth)))

	// --- I want to... section ---
	sections = append(sections, renderIntentsSectionWithState(intents, len(recs), cursor, innerWidth, executing, executingIndex, results))

	// --- Footer ---
	sections = append(sections, renderActionsFooter())

	content := strings.Join(sections, "\n\n")

	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorSurface1).
		Padding(1, 2).
		Width(maxWidth)

	return boxStyle.Render(content)
}

// renderRecsSection renders the "Needs attention" section (backward-compatible).
func renderRecsSection(recs []recommendation, cursor int, innerWidth int) string {
	return renderRecsSectionWithState(recs, cursor, innerWidth, false, -1, nil)
}

// renderRecsSectionWithState renders the "Needs attention" section with execution state.
func renderRecsSectionWithState(recs []recommendation, cursor int, innerWidth int,
	executing bool, executingIndex int, results map[int]actionResultMsg) string {
	headerStyle := lipgloss.NewStyle().Bold(true).Foreground(colorText)
	dimStyle := lipgloss.NewStyle().Foreground(colorSubtext0)
	greenStyle := lipgloss.NewStyle().Foreground(colorGreen)
	redStyle := lipgloss.NewStyle().Foreground(colorRed)
	boldBlue := lipgloss.NewStyle().Bold(true).Foreground(colorBlue)
	yellowStyle := lipgloss.NewStyle().Foreground(colorYellow)
	textStyle := lipgloss.NewStyle().Foreground(colorText)

	var lines []string
	lines = append(lines, headerStyle.Render("Needs attention"))
	lines = append(lines, "")

	if len(recs) == 0 {
		lines = append(lines, greenStyle.Render("\u2713 Everything looks good"))
		return strings.Join(lines, "\n")
	}

	for i, rec := range recs {
		// Recommendation title with icon
		lines = append(lines, textStyle.Render(fmt.Sprintf("%s %s", rec.icon, rec.title)))

		// Optional detail line
		if rec.detail != "" {
			lines = append(lines, dimStyle.Render("  "+rec.detail))
		}

		// Check for execution state on this item
		if result, hasResult := results[i]; hasResult {
			// Show result
			if result.success {
				lines = append(lines, greenStyle.Render("\u2713 "+result.message))
			} else {
				errMsg := result.message
				if errMsg == "" && result.err != nil {
					errMsg = result.err.Error()
				}
				lines = append(lines, redStyle.Render("\u2717 "+errMsg))
			}
		} else if executing && executingIndex == i {
			// Show executing spinner
			lines = append(lines, yellowStyle.Render("\u27f3 "+rec.action.label+"..."))
		} else {
			// Normal action line (selectable)
			hint := "enter"
			if !rec.action.inline {
				hint = "\u2192"
			}

			if cursor == i {
				actionLine := boldBlue.Render("> " + rec.action.label)
				hintText := dimStyle.Render(hint)
				gap := innerWidth - lipgloss.Width(actionLine) - lipgloss.Width(hintText)
				if gap < 1 {
					gap = 1
				}
				lines = append(lines, actionLine+strings.Repeat(" ", gap)+hintText)
			} else {
				actionLine := dimStyle.Render("  " + rec.action.label)
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

// renderIntentsSection renders the "I want to..." section (backward-compatible).
func renderIntentsSection(intents []intent, recCount, cursor int, innerWidth int) string {
	return renderIntentsSectionWithState(intents, recCount, cursor, innerWidth, false, -1, nil)
}

// renderIntentsSectionWithState renders the "I want to..." section with execution state.
func renderIntentsSectionWithState(intents []intent, recCount, cursor int, innerWidth int,
	executing bool, executingIndex int, results map[int]actionResultMsg) string {
	headerStyle := lipgloss.NewStyle().Bold(true).Foreground(colorText)
	dimStyle := lipgloss.NewStyle().Foreground(colorSubtext0)
	greenStyle := lipgloss.NewStyle().Foreground(colorGreen)
	redStyle := lipgloss.NewStyle().Foreground(colorRed)
	boldBlue := lipgloss.NewStyle().Bold(true).Foreground(colorBlue)
	yellowStyle := lipgloss.NewStyle().Foreground(colorYellow)
	textStyle := lipgloss.NewStyle().Foreground(colorText)

	var lines []string
	lines = append(lines, headerStyle.Render("I want to..."))
	lines = append(lines, "")

	for i, it := range intents {
		globalIdx := recCount + i
		isSelected := cursor == globalIdx

		// Check for execution state on this item
		if result, hasResult := results[globalIdx]; hasResult {
			// Show result
			if result.success {
				lines = append(lines, greenStyle.Render("\u2713 "+result.message))
			} else {
				errMsg := result.message
				if errMsg == "" && result.err != nil {
					errMsg = result.err.Error()
				}
				lines = append(lines, redStyle.Render("\u2717 "+errMsg))
			}
			continue
		}

		if executing && executingIndex == globalIdx {
			// Show executing spinner
			lines = append(lines, yellowStyle.Render("\u27f3 "+it.action.label+"..."))
			continue
		}

		if isSelected {
			label := boldBlue.Render("> " + it.label)
			hintText := dimStyle.Render(it.hint)
			gap := innerWidth - lipgloss.Width(label) - lipgloss.Width(hintText)
			if gap < 1 {
				gap = 1
			}
			lines = append(lines, label+strings.Repeat(" ", gap)+hintText)
		} else {
			label := textStyle.Render("  " + it.label)
			hintText := dimStyle.Render(it.hint)
			gap := innerWidth - lipgloss.Width(label) - lipgloss.Width(hintText)
			if gap < 1 {
				gap = 1
			}
			lines = append(lines, label+strings.Repeat(" ", gap)+hintText)
		}
	}

	return strings.Join(lines, "\n")
}

// renderActionsFooter renders the keyboard shortcut hints for the actions view.
func renderActionsFooter() string {
	dimStyle := lipgloss.NewStyle().Foreground(colorSubtext0)
	return dimStyle.Render("/ filter  ? help  esc dashboard  q quit")
}
