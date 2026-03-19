package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/ruminaider/claude-sync/internal/commands"
)

// renderDashboard renders the full status dashboard from MenuState.
// It returns a styled string suitable for display in a terminal.
func renderDashboard(state commands.MenuState, width, height int, version string, freshInstallCursor int) string {
	if !state.ConfigExists {
		return renderFreshInstall(width, height, version, freshInstallCursor)
	}

	maxWidth := width - 2
	if maxWidth > 70 {
		maxWidth = 70
	}
	if maxWidth < 30 {
		maxWidth = 30
	}

	var sections []string

	sections = append(sections, renderHeader(state, version))
	sections = append(sections, renderUserConfigSection(state))
	sections = append(sections, renderProjectSection(state))
	sections = append(sections, renderSyncSection(state))
	sections = append(sections, renderPluginsSection(state))
	sections = append(sections, renderFooter())

	content := strings.Join(sections, "\n\n")

	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorSurface0).
		Padding(1, 2).
		Width(maxWidth)

	return boxStyle.Render(content)
}

// renderHeader renders just the title line.
func renderHeader(state commands.MenuState, version string) string {
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(colorBlue)
	dimStyle := lipgloss.NewStyle().Foreground(colorOverlay0)
	return titleStyle.Render("claude-sync") + " " + dimStyle.Render("v"+version)
}

// renderUserConfigSection renders the user-level configuration status.
func renderUserConfigSection(state commands.MenuState) string {
	header := sectionHeader("User Config")
	labelStyle := lipgloss.NewStyle().Foreground(colorPeach)
	valueStyle := lipgloss.NewStyle().Foreground(colorText)
	tealStyle := lipgloss.NewStyle().Foreground(colorTeal)
	greenStyle := lipgloss.NewStyle().Foreground(colorGreen)
	yellowStyle := lipgloss.NewStyle().Foreground(colorYellow)
	dimStyle := lipgloss.NewStyle().Foreground(colorSubtext0)

	var lines []string
	lines = append(lines, header)

	// Config repo
	if state.ConfigRepo != "" {
		lines = append(lines, labelStyle.Render("Config repo  ")+
			valueStyle.Render(state.ConfigRepo)+"  "+tealStyle.Render("✓ connected"))
	} else {
		lines = append(lines, labelStyle.Render("Config repo  ")+
			yellowStyle.Render("not configured"))
	}

	// Active profile
	if state.ActiveProfile != "" {
		lines = append(lines, labelStyle.Render("Profile      ")+
			greenStyle.Render("● ")+valueStyle.Render(state.ActiveProfile))
	} else if len(state.Profiles) > 0 {
		lines = append(lines, labelStyle.Render("Profile      ")+
			valueStyle.Render("base (default)")+"  "+
			dimStyle.Render(fmt.Sprintf("%d other profiles available", len(state.Profiles))))
	} else {
		lines = append(lines, labelStyle.Render("Profile      ")+
			valueStyle.Render("base (default)"))
	}

	// Plugin count
	lines = append(lines, labelStyle.Render("Plugins      ")+
		valueStyle.Render(fmt.Sprintf("%d installed", len(state.Plugins))))

	return strings.Join(lines, "\n")
}

// renderSyncSection renders the sync status lines.
func renderSyncSection(state commands.MenuState) string {
	tealStyle := lipgloss.NewStyle().Foreground(colorTeal)
	yellowStyle := lipgloss.NewStyle().Foreground(colorYellow)
	redStyle := lipgloss.NewStyle().Foreground(colorRed)
	maroonStyle := lipgloss.NewStyle().Foreground(colorMaroon)

	header := sectionHeader("Sync")
	var lines []string
	lines = append(lines, header)

	if state.HasConflicts {
		lines = append(lines, redStyle.Render("✗ Merge conflicts need resolution"))
	}

	if state.CommitsBehind > 0 {
		msg := fmt.Sprintf("⚠ Config is %d commits behind", state.CommitsBehind)
		lines = append(lines, yellowStyle.Render(msg))
	} else {
		lines = append(lines, tealStyle.Render("✓ Config up to date"))
	}

	if state.HasPending {
		lines = append(lines, maroonStyle.Render("⚠ pending change(s) awaiting approval"))
	}

	return strings.Join(lines, "\n")
}

// renderPluginsSection renders the plugins list with untracked detection.
func renderPluginsSection(state commands.MenuState) string {
	header := sectionHeader(fmt.Sprintf("Active Plugins (%d)", len(state.Plugins)))

	if len(state.Plugins) == 0 && len(state.UntrackedPlugins) == 0 {
		dimStyle := lipgloss.NewStyle().Foreground(colorSubtext0)
		return header + "\n" + dimStyle.Render("No plugins configured")
	}

	// Calculate max name width for alignment (across both synced and untracked)
	maxNameLen := 0
	for _, p := range state.Plugins {
		if len(p.Name) > maxNameLen {
			maxNameLen = len(p.Name)
		}
	}
	for _, key := range state.UntrackedPlugins {
		name := key
		if idx := strings.Index(key, "@"); idx >= 0 {
			name = key[:idx]
		}
		if len(name) > maxNameLen {
			maxNameLen = len(name)
		}
	}

	labelStyle := lipgloss.NewStyle().Foreground(colorSubtext0)
	nameStyle := lipgloss.NewStyle().Foreground(colorMauve)
	blueStyle := lipgloss.NewStyle().Foreground(colorBlue)
	pinkStyle := lipgloss.NewStyle().Foreground(colorPink)
	yellowStyle := lipgloss.NewStyle().Foreground(colorYellow)
	peachStyle := lipgloss.NewStyle().Foreground(colorPeach)

	var lines []string
	lines = append(lines, header)

	for _, p := range state.Plugins {
		name := fmt.Sprintf("%-*s", maxNameLen, p.Name)
		var statusTag, extra string

		switch p.Status {
		case "upstream":
			statusTag = labelStyle.Render("upstream")
			if p.Marketplace != "" {
				extra = labelStyle.Render(p.Marketplace)
			}
		case "pinned":
			statusTag = blueStyle.Render("pinned")
			extra = labelStyle.Render("v" + p.PinVersion)
			if p.LatestVersion != "" {
				extra += peachStyle.Render(" (latest: v" + p.LatestVersion + ")")
			}
		case "forked":
			statusTag = pinkStyle.Render("forked")
			extra = labelStyle.Render("local edits")
		default:
			statusTag = labelStyle.Render(p.Status)
		}

		line := "  " + nameStyle.Render(name) + "  " + statusTag
		if extra != "" {
			line += "  " + extra
		}
		lines = append(lines, line)
	}

	// Untracked plugins subsection
	if len(state.UntrackedPlugins) > 0 {
		lines = append(lines, "")
		lines = append(lines, yellowStyle.Render("Not in synced config:"))
		for _, key := range state.UntrackedPlugins {
			name := key
			if idx := strings.Index(key, "@"); idx >= 0 {
				name = key[:idx]
			}
			paddedName := fmt.Sprintf("%-*s", maxNameLen, name)
			lines = append(lines, "  "+yellowStyle.Render(paddedName)+"  "+
				labelStyle.Render("installed locally"))
		}
	}

	return strings.Join(lines, "\n")
}

// renderProjectSection renders the "This Project" section.
func renderProjectSection(state commands.MenuState) string {
	header := sectionHeader("This Project")
	labelStyle := lipgloss.NewStyle().Foreground(colorPeach)
	valueStyle := lipgloss.NewStyle().Foreground(colorText)
	dimStyle := lipgloss.NewStyle().Foreground(colorSubtext0)
	yellowStyle := lipgloss.NewStyle().Foreground(colorYellow)
	greenStyle := lipgloss.NewStyle().Foreground(colorGreen)

	if state.ProjectDir == "" {
		return header + "\n" + dimStyle.Render("Not in a project directory")
	}

	shortPath := shortenPath(state.ProjectDir)
	var lines []string
	lines = append(lines, header)
	lines = append(lines, labelStyle.Render("Path         ")+valueStyle.Render(shortPath))

	if !state.ProjectInitialized {
		lines = append(lines, yellowStyle.Render("⚠ No settings profile assigned to this project"))
		lines = append(lines, dimStyle.Render("  Using your base config. Assign a profile to customize."))
		return strings.Join(lines, "\n")
	}

	// Show which settings profile applies
	if state.ProjectProfile != "" {
		lines = append(lines, labelStyle.Render("Profile      ")+
			greenStyle.Render("● ")+valueStyle.Render(state.ProjectProfile))
	} else {
		lines = append(lines, labelStyle.Render("Profile      ")+
			valueStyle.Render("base (default)"))
	}

	// CLAUDE.md and MCP counts
	mdInfo := fmt.Sprintf("%d synced section", state.ClaudeMDCount)
	if state.ClaudeMDCount != 1 {
		mdInfo += "s"
	}
	mcpInfo := fmt.Sprintf("%d server", state.MCPCount)
	if state.MCPCount != 1 {
		mcpInfo += "s"
	}
	lines = append(lines, labelStyle.Render("CLAUDE.md    ")+valueStyle.Render(mdInfo)+
		labelStyle.Render("  │  ")+labelStyle.Render("MCP  ")+valueStyle.Render(mcpInfo))

	return strings.Join(lines, "\n")
}

// renderFooter renders the keyboard shortcut hints.
func renderFooter() string {
	keyStyle := lipgloss.NewStyle().Bold(true).Foreground(colorMauve)
	hintStyle := lipgloss.NewStyle().Foreground(colorOverlay0)

	return keyStyle.Render("enter") + hintStyle.Render(" see what you can do") +
		"    " + hintStyle.Render("q quit")
}

// renderFreshInstall renders the welcome screen for a fresh installation.
// cursor indicates which option is selected: 0 = Create, 1 = Join.
func renderFreshInstall(width, height int, version string, cursor int) string {
	maxWidth := width - 2
	if maxWidth > 70 {
		maxWidth = 70
	}
	if maxWidth < 30 {
		maxWidth = 30
	}

	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(colorBlue)
	dimStyle := lipgloss.NewStyle().Foreground(colorSubtext0)
	textStyle := lipgloss.NewStyle().Foreground(colorText)
	selectedStyle := lipgloss.NewStyle().Bold(true).Foreground(colorBlue)

	title := titleStyle.Render("claude-sync") + " " + dimStyle.Render("v"+version)

	// Build option lines with cursor indicator
	createPrefix := "  "
	createLabel := textStyle.Render("Create new config")
	joinPrefix := "  "
	joinLabel := textStyle.Render("Join a shared config")

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
	lines = append(lines, textStyle.Render("No config found. Get started:"))
	lines = append(lines, "")
	lines = append(lines, textStyle.Render(createPrefix)+createLabel+"  "+dimStyle.Render("from this machine's Claude Code setup"))
	lines = append(lines, textStyle.Render(joinPrefix)+joinLabel+"  "+dimStyle.Render("clone a shared config repo"))
	lines = append(lines, "")
	lines = append(lines, dimStyle.Render("↑↓ navigate  enter select  q quit"))

	content := strings.Join(lines, "\n")

	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorSurface0).
		Padding(1, 2).
		Width(maxWidth)

	return boxStyle.Render(content)
}

// sectionHeader renders a section divider with a label.
func sectionHeader(label string) string {
	lineStyle := lipgloss.NewStyle().Foreground(colorSurface1)
	labelStyle := lipgloss.NewStyle().Foreground(colorPink).Bold(true)
	return lineStyle.Render("── ") + labelStyle.Render(label) + " " + lineStyle.Render(strings.Repeat("─", 40))
}

