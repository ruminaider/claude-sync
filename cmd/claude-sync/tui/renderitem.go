package tui

import "github.com/charmbracelet/lipgloss"

// RenderCheckbox returns a styled [x] or [ ] checkbox.
// When focused, uses SelectedStyle/UnselectedStyle; otherwise DimStyle.
func RenderCheckbox(focused, selected bool) string {
	if focused {
		if selected {
			return SelectedStyle.Render("[x]")
		}
		return UnselectedStyle.Render("[ ]")
	}
	if selected {
		return DimStyle.Render("[x]")
	}
	return DimStyle.Render("[ ]")
}

// RenderItemText returns styled display text for a list item.
// Bold+colorText when current, plain when focused, DimStyle when unfocused.
func RenderItemText(text string, focused, isCurrent bool) string {
	if isCurrent {
		return lipgloss.NewStyle().Bold(true).Foreground(colorText).Render(text)
	}
	if focused {
		return text
	}
	return DimStyle.Render(text)
}

// RenderTag returns a styled tag string (e.g. "●", "[cmd]").
// Empty tag returns "". Custom tagColor applies a foreground color;
// empty tagColor uses BaseTagStyle. Unfocused uses DimStyle.
func RenderTag(tag string, focused bool, tagColor lipgloss.Color) string {
	if tag == "" {
		return ""
	}
	if focused && tagColor != "" {
		return "  " + lipgloss.NewStyle().Foreground(tagColor).Render(tag)
	}
	if focused {
		return "  " + BaseTagStyle.Render(tag)
	}
	return "  " + DimStyle.Render(tag)
}

// RenderHeader returns a styled header line. Callers format the line content
// (e.g. "── ▾ title ──"); this function applies HeaderStyle/bold/DimStyle.
func RenderHeader(line string, focused, isCurrent bool) string {
	if isCurrent {
		return lipgloss.NewStyle().Bold(true).Foreground(colorText).Render(line)
	}
	if focused {
		return HeaderStyle.Render(line)
	}
	return DimStyle.Render(line)
}

// RenderRemovedBaseLine renders an inherited item that the user has deselected
// (explicitly removed from this profile). The entire line is shown in maroon
// with strikethrough when focused; falls back to DimStyle when unfocused.
func RenderRemovedBaseLine(display, tag string, focused bool) string {
	content := "[ ] " + display
	if tag != "" {
		content += "  " + tag
	}
	if focused {
		return RemovedBaseStyle.Render(content)
	}
	return DimStyle.Render(content)
}

// RenderLockedLine renders a read-only item with a [-] checkbox indicating it
// is plugin-controlled and cannot be toggled. Display and tag use LockedStyle.
func RenderLockedLine(display, tag string, focused bool) string {
	if focused {
		checkbox := LockedStyle.Render("[-]")
		content := LockedStyle.Render(display)
		t := ""
		if tag != "" {
			t = "  " + LockedStyle.Render(tag)
		}
		return checkbox + " " + content + t
	}
	content := "[-] " + display
	if tag != "" {
		content += "  " + tag
	}
	return DimStyle.Render(content)
}

// RenderSearchAction returns styled text for the search action row.
// Shows "Searching..." when searching, "[↻ Re-scan]" otherwise.
func RenderSearchAction(focused, isCurrent, searching bool) string {
	if searching {
		text := "Searching..."
		if isCurrent {
			return lipgloss.NewStyle().Bold(true).Foreground(colorOverlay0).Render(text)
		}
		return DimStyle.Render(text)
	}
	text := "[↻ Re-scan]"
	if isCurrent {
		return lipgloss.NewStyle().Bold(true).Foreground(colorBlue).Render(text)
	}
	if focused {
		return lipgloss.NewStyle().Foreground(colorBlue).Render(text)
	}
	return DimStyle.Render(text)
}
