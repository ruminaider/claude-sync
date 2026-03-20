package tui

import "strings"

// recalcScroll recalculates maxScroll and clamps scroll after a resize or content change.
// Returns the updated (scroll, maxScroll) pair.
func recalcScroll(content string, height, scroll int) (newScroll, maxScroll int) {
	allLines := strings.Split(content, "\n")
	innerHeight := height - 6 // border (2) + padding (2) + footer (2)
	if innerHeight < 1 {
		innerHeight = 1
	}
	maxScroll = len(allLines) - innerHeight
	if maxScroll < 0 {
		maxScroll = 0
	}
	newScroll = scroll
	if newScroll > maxScroll {
		newScroll = maxScroll
	}
	return newScroll, maxScroll
}

// renderScrollable renders pre-built content with scroll windowing inside a content box.
func renderScrollable(content string, width, height, scroll int) string {
	maxWidth, _ := clampWidth(width)

	allLines := strings.Split(content, "\n")
	innerHeight := height - 6
	if innerHeight < 1 {
		innerHeight = 1
	}

	start := scroll
	end := start + innerHeight
	if end > len(allLines) {
		end = len(allLines)
	}
	if start > end {
		start = end
	}

	visibleLines := allLines[start:end]

	var output []string
	output = append(output, strings.Join(visibleLines, "\n"))
	output = append(output, "")
	output = append(output, stDim.Render("j/k scroll  esc back"))

	boxContent := strings.Join(output, "\n")
	return contentBox(maxWidth, colorSurface1).Render(boxContent)
}
