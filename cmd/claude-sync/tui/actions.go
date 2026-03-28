package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/ruminaider/claude-sync/internal/commands"
)

// intent represents a goal-oriented action the user can take.
// Items with isHeader=true are non-selectable section dividers.
type intent struct {
	hint     string // shown on right: "⏎ pull" or "→"
	action   actionItem
	isHeader bool   // section divider (not selectable)
	label    string // display label (used for headers)
}

// buildIntents returns the grouped list of intent-based actions.
// The list mirrors the menu_items.go design: Sync, Plugins, Config sections.
func buildIntents(state commands.MenuState) []intent {
	return []intent{
		{label: "Sync", isHeader: true},
		{hint: "⏎ pull", action: actionItem{id: ActionPull, label: "Pull latest updates", inline: true}},
		{hint: "⏎ push", action: actionItem{id: ActionPushChanges, label: "Push local changes", inline: true}},

		{label: "Plugins", isHeader: true},
		{action: actionItem{id: ActionBrowsePlugins, label: "Browse & install plugins"}},
		{action: actionItem{id: ActionRemovePlugin, label: "Remove a plugin", inline: true}},
		{action: actionItem{id: ActionForkPlugin, label: "Fork or customize a plugin", inline: true}},
		{action: actionItem{id: ActionPinPlugin, label: "Pin a plugin version", inline: true}},

		{label: "Config", isHeader: true},
		{action: actionItem{id: ActionProfileList, label: "Switch profile"}},
		{action: actionItem{id: ActionConfigUpdate, label: "Edit full config"}},
		{action: actionItem{id: ActionSubscribe, label: "Subscribe to another config"}},
	}
}

// rawItemCount returns the total number of positions (recs + all intents
// including headers). The cursor ranges over [0, rawItemCount).
func rawItemCount(recs []recommendation, intents []intent) int {
	return len(recs) + len(intents)
}

// actionItemCount returns the total number of selectable items across both
// sections. Headers are excluded.
func actionItemCount(recs []recommendation, intents []intent) int {
	count := len(recs)
	for _, it := range intents {
		if !it.isHeader {
			count++
		}
	}
	return count
}

// selectedAction returns the actionItem at the given cursor position.
// Cursor positions 0..len(recs)-1 address recommendations.
// Positions len(recs)..len(recs)+len(intents)-1 address intents directly
// (including headers, but headers return nil since they have no action).
func selectedAction(recs []recommendation, intents []intent, cursor int) *actionItem {
	if cursor < len(recs) {
		return &recs[cursor].action
	}
	intentIdx := cursor - len(recs)
	if intentIdx >= 0 && intentIdx < len(intents) {
		if intents[intentIdx].isHeader {
			return nil // headers are not selectable
		}
		return &intents[intentIdx].action
	}
	return nil
}

// isIntentHeader returns true if the cursor position corresponds to a header
// intent. Positions in the recommendation range always return false.
func isIntentHeader(recs []recommendation, intents []intent, cursor int) bool {
	if cursor < len(recs) {
		return false
	}
	intentIdx := cursor - len(recs)
	if intentIdx >= 0 && intentIdx < len(intents) {
		return intents[intentIdx].isHeader
	}
	return false
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
				if help := ErrorGuidance(result.actionID, result.err); help != nil {
					lines = append(lines, stDim.Render("  "+help.Why))
					lines = append(lines, stYellow.Render("  \u2192 "+help.Action))
				}
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

// renderIntentsSectionWithState renders the grouped intent sections with execution state.
// Section headers replace the old "I want to..." heading.
func renderIntentsSectionWithState(intents []intent, recCount, cursor int, innerWidth int,
	executing bool, executingActionID string, results map[string]actionResultMsg) string {
	var lines []string

	for i, it := range intents {
		// Render section headers as dividers
		if it.isHeader {
			// Add blank line before headers (except the very first intent)
			if i > 0 {
				lines = append(lines, "")
			}
			lines = append(lines, sectionHeader(it.label))
			continue
		}

		globalIdx := recCount + i
		isSelected := cursor == globalIdx

		// Check for execution state on this item
		if result, hasResult := results[it.action.id]; hasResult {
			if result.success {
				lines = append(lines, stGreen.Render("\u2713 "+result.message))
			} else {
				errMsg := result.message
				if errMsg == "" && result.err != nil {
					errMsg = result.err.Error()
				}
				lines = append(lines, stRed.Render("\u2717 "+errMsg))
				if help := ErrorGuidance(result.actionID, result.err); help != nil {
					lines = append(lines, stDim.Render("  "+help.Why))
					lines = append(lines, stYellow.Render("  \u2192 "+help.Action))
				}
			}
			continue
		}

		if executing && executingActionID == it.action.id {
			lines = append(lines, stYellow.Render("\u27f3 "+it.action.label+"..."))
			continue
		}

		if isSelected {
			label := stBlue.Render("> " + it.action.label)
			hintText := stDim.Render(it.hint)
			gap := innerWidth - lipgloss.Width(label) - lipgloss.Width(hintText)
			if gap < 1 {
				gap = 1
			}
			lines = append(lines, label+strings.Repeat(" ", gap)+hintText)
		} else {
			label := stText.Render("  " + it.action.label)
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

	// --- Grouped intent sections ---
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
// Headers always pass through (they are structural, not filterable).
// A header is kept only if at least one item in its section matches.
func filterIntents(intents []intent, query string) []intent {
	if query == "" {
		return intents
	}
	query = strings.ToLower(query)

	// First pass: identify which sections have matching items.
	// A "section" starts at each header and runs until the next header.
	type section struct {
		headerIdx int // index of header in intents (-1 if no header before first items)
		hasMatch  bool
	}
	var sections []section
	currentSection := -1
	for i, it := range intents {
		if it.isHeader {
			sections = append(sections, section{headerIdx: i})
			currentSection = len(sections) - 1
			continue
		}
		if strings.Contains(strings.ToLower(it.action.label), query) {
			if currentSection >= 0 {
				sections[currentSection].hasMatch = true
			}
		}
	}

	// Build a set of header indices that have matches.
	keepHeader := map[int]bool{}
	for _, s := range sections {
		if s.hasMatch {
			keepHeader[s.headerIdx] = true
		}
	}

	// Second pass: emit kept headers and matching items.
	var filtered []intent
	for i, it := range intents {
		if it.isHeader {
			if keepHeader[i] {
				filtered = append(filtered, it)
			}
			continue
		}
		if strings.Contains(strings.ToLower(it.action.label), query) {
			filtered = append(filtered, it)
		}
	}
	return filtered
}
