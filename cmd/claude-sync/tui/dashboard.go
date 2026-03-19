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
	sections = append(sections, renderProfilesSection(state))
	sections = append(sections, renderFooter())

	content := strings.Join(sections, "\n\n")

	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorSurface1).
		Padding(1, 2).
		Width(maxWidth)

	return boxStyle.Render(content)
}

// renderHeader renders just the title line.
func renderHeader(state commands.MenuState, version string) string {
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(colorBlue)
	dimStyle := lipgloss.NewStyle().Foreground(colorSubtext0)
	return titleStyle.Render("claude-sync") + " " + dimStyle.Render("v"+version)
}

// renderUserConfigSection renders the user-level configuration status.
func renderUserConfigSection(state commands.MenuState) string {
	header := sectionHeader("User Config")
	dimStyle := lipgloss.NewStyle().Foreground(colorSubtext0)
	textStyle := lipgloss.NewStyle().Foreground(colorText)
	greenStyle := lipgloss.NewStyle().Foreground(colorGreen)
	yellowStyle := lipgloss.NewStyle().Foreground(colorYellow)

	var lines []string
	lines = append(lines, header)

	// Config repo
	if state.ConfigRepo != "" {
		lines = append(lines, textStyle.Render("Config repo: ")+dimStyle.Render(state.ConfigRepo)+
			"  "+greenStyle.Render("✓ connected"))
	} else {
		lines = append(lines, textStyle.Render("Config repo: ")+
			yellowStyle.Render("not configured"))
	}

	// Active profile
	if state.ActiveProfile != "" {
		lines = append(lines, textStyle.Render("Active profile: ")+
			greenStyle.Render("● ")+textStyle.Render(state.ActiveProfile))
	} else if len(state.Profiles) > 0 {
		lines = append(lines, textStyle.Render("Active profile: ")+
			dimStyle.Render("none")+
			dimStyle.Render(fmt.Sprintf("  (%d available)", len(state.Profiles))))
	} else {
		lines = append(lines, textStyle.Render("Active profile: ")+
			dimStyle.Render("none"))
	}

	// Plugin + settings summary
	lines = append(lines, textStyle.Render("Plugins: ")+
		dimStyle.Render(fmt.Sprintf("%d installed", len(state.Plugins))))

	return strings.Join(lines, "\n")
}

// renderSyncSection renders the sync status lines.
func renderSyncSection(state commands.MenuState) string {
	greenStyle := lipgloss.NewStyle().Foreground(colorGreen)
	yellowStyle := lipgloss.NewStyle().Foreground(colorYellow)
	redStyle := lipgloss.NewStyle().Foreground(colorRed)

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
		lines = append(lines, greenStyle.Render("✓ Config up to date"))
	}

	if state.HasPending {
		lines = append(lines, yellowStyle.Render("⚠ pending change(s) awaiting approval"))
	}

	return strings.Join(lines, "\n")
}

// renderPluginsSection renders the plugins list.
func renderPluginsSection(state commands.MenuState) string {
	header := sectionHeader(fmt.Sprintf("Plugins (%d)", len(state.Plugins)))

	if len(state.Plugins) == 0 {
		dimStyle := lipgloss.NewStyle().Foreground(colorSubtext0)
		return header + "\n" + dimStyle.Render("No plugins configured")
	}

	// Calculate max name width for alignment
	maxNameLen := 0
	for _, p := range state.Plugins {
		if len(p.Name) > maxNameLen {
			maxNameLen = len(p.Name)
		}
	}

	dimStyle := lipgloss.NewStyle().Foreground(colorSubtext0)
	blueStyle := lipgloss.NewStyle().Foreground(colorBlue)
	greenStyle := lipgloss.NewStyle().Foreground(colorGreen)

	var lines []string
	lines = append(lines, header)

	for _, p := range state.Plugins {
		name := fmt.Sprintf("%-*s", maxNameLen, p.Name)
		var statusTag, extra string

		switch p.Status {
		case "upstream":
			statusTag = dimStyle.Render("upstream")
			if p.Marketplace != "" {
				extra = dimStyle.Render(p.Marketplace)
			}
		case "pinned":
			statusTag = blueStyle.Render("pinned")
			extra = dimStyle.Render("v" + p.PinVersion)
			if p.LatestVersion != "" {
				extra += dimStyle.Render(" (latest: v" + p.LatestVersion + ")")
			}
		case "forked":
			statusTag = greenStyle.Render("forked")
			extra = dimStyle.Render("local edits")
		default:
			statusTag = dimStyle.Render(p.Status)
		}

		line := "  " + lipgloss.NewStyle().Foreground(colorText).Render(name) +
			"  " + statusTag
		if extra != "" {
			line += "  " + extra
		}
		lines = append(lines, line)
	}

	return strings.Join(lines, "\n")
}

// renderProfilesSection renders the profiles list.
func renderProfilesSection(state commands.MenuState) string {
	header := sectionHeader(fmt.Sprintf("Profiles (%d)", len(state.Profiles)))

	if len(state.Profiles) == 0 {
		dimStyle := lipgloss.NewStyle().Foreground(colorSubtext0)
		return header + "\n" + dimStyle.Render("No profiles configured")
	}

	greenStyle := lipgloss.NewStyle().Foreground(colorGreen)
	textStyle := lipgloss.NewStyle().Foreground(colorText)
	dimStyle := lipgloss.NewStyle().Foreground(colorSubtext0)

	var lines []string
	lines = append(lines, header)

	for _, p := range state.Profiles {
		if p == state.ActiveProfile {
			marker := greenStyle.Render("●")
			name := greenStyle.Render(p)
			suffix := dimStyle.Render(" (active)")
			lines = append(lines, "  "+marker+" "+name+suffix)
		} else {
			lines = append(lines, "    "+textStyle.Render(p))
		}
	}

	return strings.Join(lines, "\n")
}

// renderProjectSection renders the "This Project" section.
func renderProjectSection(state commands.MenuState) string {
	header := sectionHeader("This Project")
	dimStyle := lipgloss.NewStyle().Foreground(colorSubtext0)
	textStyle := lipgloss.NewStyle().Foreground(colorText)
	yellowStyle := lipgloss.NewStyle().Foreground(colorYellow)
	greenStyle := lipgloss.NewStyle().Foreground(colorGreen)

	if state.ProjectDir == "" {
		return header + "\n" + dimStyle.Render("Not in a project directory")
	}

	shortPath := shortenPath(state.ProjectDir)
	var lines []string
	lines = append(lines, header)
	lines = append(lines, textStyle.Render("Path: ")+dimStyle.Render(shortPath))

	if !state.ProjectInitialized {
		lines = append(lines, yellowStyle.Render("⚠ No settings profile assigned to this project"))
		lines = append(lines, dimStyle.Render("  Using your base config. Assign a profile to customize."))
		return strings.Join(lines, "\n")
	}

	// Show which settings profile applies
	if state.ProjectProfile != "" {
		lines = append(lines, textStyle.Render("Settings profile: ")+
			greenStyle.Render("● ")+textStyle.Render(state.ProjectProfile))
	} else {
		lines = append(lines, textStyle.Render("Settings profile: ")+
			dimStyle.Render("base (default)"))
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
	lines = append(lines, textStyle.Render("CLAUDE.md: ")+dimStyle.Render(mdInfo)+
		textStyle.Render(" │ MCP: ")+dimStyle.Render(mcpInfo))

	return strings.Join(lines, "\n")
}

// renderFooter renders the keyboard shortcut hints.
func renderFooter() string {
	dimStyle := lipgloss.NewStyle().Foreground(colorSubtext0)
	return dimStyle.Render("enter actions  q quit")
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
		BorderForeground(colorSurface1).
		Padding(1, 2).
		Width(maxWidth)

	return boxStyle.Render(content)
}

// sectionHeader renders a section divider with a label.
func sectionHeader(label string) string {
	dimStyle := lipgloss.NewStyle().Foreground(colorSurface1)
	textStyle := lipgloss.NewStyle().Foreground(colorText).Bold(true)
	return dimStyle.Render("── ") + textStyle.Render(label) + " " + dimStyle.Render(strings.Repeat("─", 40))
}

