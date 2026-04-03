package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/ruminaider/claude-sync/internal/commands"
)

// renderSummary renders the compact status summary lines for the top of the main screen.
// These lines are NOT selectable — they're informational only.
func renderSummary(state commands.MenuState, version string) string {
	labelStyle := lipgloss.NewStyle().Foreground(colorPeach)
	valueStyle := lipgloss.NewStyle().Foreground(colorText)
	tealStyle := lipgloss.NewStyle().Foreground(colorTeal)

	var lines []string

	// User config line
	if state.ConfigRepo != "" {
		var syncStatus string
		switch {
		case state.CommitsBehind < 0:
			syncStatus = stDim.Render("? sync unknown")
		case state.CommitsBehind > 0:
			syncStatus = stYellow.Render(fmt.Sprintf("⚠ %d behind", state.CommitsBehind))
		default:
			syncStatus = tealStyle.Render("✓ synced")
		}
		lines = append(lines, labelStyle.Render("User config     ")+
			valueStyle.Render(state.ConfigRepo)+"  "+syncStatus)
	} else {
		lines = append(lines, labelStyle.Render("User config     ")+
			stYellow.Render("not configured"))
	}

	// User profile line
	if state.ActiveProfile != "" {
		lines = append(lines, labelStyle.Render("User profile    ")+
			valueStyle.Render(state.ActiveProfile))
	} else if len(state.Profiles) > 0 {
		lines = append(lines, labelStyle.Render("User profile    ")+
			valueStyle.Render("base (default)")+"  "+
			stDim.Render(fmt.Sprintf("%d others available", len(state.Profiles))))
	} else {
		lines = append(lines, labelStyle.Render("User profile    ")+
			valueStyle.Render("base (default)"))
	}

	// Active plugins line
	lines = append(lines, labelStyle.Render("Active plugins  ")+
		valueStyle.Render(fmt.Sprintf("%d installed", len(state.Plugins))))

	// Blank line
	lines = append(lines, "")

	// This project line
	if state.ProjectDir != "" {
		shortPath := shortenPath(state.ProjectDir)
		lines = append(lines, labelStyle.Render("This project    ")+valueStyle.Render(shortPath))

		if !state.ProjectInitialized {
			lines = append(lines, stYellow.Render("⚠ No settings profile assigned to this project"))
		} else if state.ProjectProfile != "" {
			lines = append(lines, labelStyle.Render("Project profile ")+
				stGreen.Render("● ")+valueStyle.Render(state.ProjectProfile))
		} else {
			lines = append(lines, labelStyle.Render("Project profile ")+
				valueStyle.Render("base (default)"))
		}
	} else {
		lines = append(lines, stDim.Render("Not in a project directory"))
	}

	return strings.Join(lines, "\n")
}

func renderUpdateBanner(currentVersion, latestVersion string, width int) string {
	borderStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorLatteMauve).
		Padding(1, 2).
		Width(width - 4)

	labelStyle := lipgloss.NewStyle().Foreground(colorLatteMauve).Bold(true)
	textStyle := lipgloss.NewStyle().Foreground(colorLatteLavender)
	cmdStyle := lipgloss.NewStyle().Foreground(colorLattePeach).Bold(true)

	title := labelStyle.Render("UPDATE AVAILABLE")
	line1 := textStyle.Render(fmt.Sprintf("Update available: %s → %s", currentVersion, latestVersion))
	line2 := textStyle.Render("Run ") + cmdStyle.Render("claude-sync update") + textStyle.Render(" to update")

	content := title + "\n\n" + line1 + "\n" + line2
	return borderStyle.Render(content)
}

// MainScreenParams groups the arguments for renderMainScreen to avoid
// positional-parameter sprawl (previously 14 args with 4 bare bools).
type MainScreenParams struct {
	State             commands.MenuState
	Recs              []recommendation
	Intents           []intent
	Cursor            int
	Width, Height     int
	Version           string
	Executing         bool
	ExecutingActionID string
	Results           map[string]actionResultMsg
	FilterMode        bool
	FilterText        string
	UpdateAvailable   bool
	LatestVersion     string
}

// renderMainScreen renders the unified main screen with summary at top,
// recommendations, and intents in one view.
func renderMainScreen(p MainScreenParams) string {

	maxWidth, innerWidth := clampWidth(p.Width)

	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(colorBlue)
	dimStyle := lipgloss.NewStyle().Foreground(colorOverlay0)

	var sections []string

	// --- Header ---
	sections = append(sections, titleStyle.Render("claude-sync")+" "+dimStyle.Render("v"+p.Version))

	if p.UpdateAvailable {
		sections = append(sections, renderUpdateBanner(p.Version, p.LatestVersion, maxWidth))
	}

	// --- Summary ---
	sections = append(sections, renderSummary(p.State, p.Version))

	// --- Filter bar (when active) ---
	if p.FilterMode || p.FilterText != "" {
		filterStyle := lipgloss.NewStyle().Foreground(colorBlue)
		cursorChar := ""
		if p.FilterMode {
			cursorChar = "\u2588" // block cursor
		}
		sections = append(sections, filterStyle.Render("/ "+p.FilterText+cursorChar))
	}

	// --- No results message ---
	if len(p.Recs) == 0 && len(p.Intents) == 0 && p.FilterText != "" {
		noMatchStyle := lipgloss.NewStyle().Foreground(colorSubtext0)
		sections = append(sections, noMatchStyle.Render("No matching actions"))
	} else {
		// --- Needs attention / Status section ---
		sections = append(sections, renderRecsSectionWithState(p.Recs, p.Cursor, innerWidth, p.Executing, p.ExecutingActionID, p.Results))

		// --- Grouped intent sections (Sync, Plugins, Config) ---
		sections = append(sections, renderIntentsSectionWithState(p.Intents, len(p.Recs), p.Cursor, innerWidth, p.Executing, p.ExecutingActionID, p.Results))
	}

	// --- Footer ---
	sections = append(sections, renderMainFooter())

	content := strings.Join(sections, "\n\n")

	boxStyle := contentBox(maxWidth, colorSurface1)

	return boxStyle.Render(content)
}

// renderMainFooter renders the keyboard shortcut hints for the main screen.
func renderMainFooter() string {
	return stDim.Render("/ filter  ? help  q quit")
}

// renderFreshInstall renders the welcome screen for a fresh installation.
// cursor indicates which option is selected: 0 = Create, 1 = Join.
func renderFreshInstall(width, height int, version string, cursor int) string {
	maxWidth, _ := clampWidth(width)

	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(colorBlue)
	selectedStyle := lipgloss.NewStyle().Bold(true).Foreground(colorBlue)

	title := titleStyle.Render("claude-sync") + " " + stDim.Render("v"+version)

	// Build option lines with cursor indicator
	createPrefix := "  "
	createLabel := stText.Render("Create new config")
	joinPrefix := "  "
	joinLabel := stText.Render("Join a shared config")

	if cursor == 0 {
		createPrefix = "> "
		createLabel = selectedStyle.Render("Create new config")
	} else {
		joinPrefix = "> "
		joinLabel = selectedStyle.Render("Join a shared config")
	}

	var lines []string
	lines = append(lines, title)
	lines = append(lines, "")
	lines = append(lines, stText.Render("No config found. Get started:"))
	lines = append(lines, "")
	lines = append(lines, stText.Render(createPrefix)+createLabel+"  "+stDim.Render("from this machine's Claude Code setup"))
	lines = append(lines, stText.Render(joinPrefix)+joinLabel+"  "+stDim.Render("clone a shared config repo"))
	lines = append(lines, "")
	lines = append(lines, stDim.Render("↑↓ navigate  enter select  q quit"))

	content := strings.Join(lines, "\n")

	boxStyle := contentBox(maxWidth, colorSurface0)

	return boxStyle.Render(content)
}

// sectionHeader renders a section divider with a label.
func sectionHeader(label string) string {
	lineStyle := lipgloss.NewStyle().Foreground(colorSurface1)
	labelStyle := lipgloss.NewStyle().Foreground(colorBlue).Bold(true)
	return lineStyle.Render("── ") + labelStyle.Render(label) + " " + lineStyle.Render(strings.Repeat("─", 40))
}
