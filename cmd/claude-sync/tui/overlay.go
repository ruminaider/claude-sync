package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// OverlayType identifies the kind of modal overlay.
type OverlayType int

const (
	OverlayConfirm   OverlayType = iota // Yes/No confirmation
	OverlayTextInput                     // Single-line text input
	OverlaySummary                       // Multi-line summary with confirm/cancel
	OverlayChoice                        // List of choices with cursor
)

// Overlay renders a centered modal box on top of existing content.
type Overlay struct {
	overlayType OverlayType
	title       string
	message     string   // body text (for Confirm, Summary)
	choices     []string // choice list (for Choice)
	cursor      int      // selected choice index (Choice), or button index (Confirm/Summary: 0=Cancel, 1=OK)
	input       textinput.Model
	width       int // overlay box width
	active      bool
}

// NewConfirmOverlay creates a confirmation dialog with Cancel/OK buttons.
func NewConfirmOverlay(title, message string) Overlay {
	return Overlay{
		overlayType: OverlayConfirm,
		title:       title,
		message:     message,
		cursor:      1, // default to OK
		active:      true,
	}
}

// NewTextInputOverlay creates a text input dialog.
func NewTextInputOverlay(title, placeholder string) Overlay {
	ti := textinput.New()
	ti.Placeholder = placeholder
	ti.Focus()
	ti.CharLimit = 64
	ti.Width = 30
	return Overlay{
		overlayType: OverlayTextInput,
		title:       title,
		input:       ti,
		active:      true,
	}
}

// NewSummaryOverlay creates a summary dialog with Cancel/Initialize buttons.
func NewSummaryOverlay(title, body string) Overlay {
	return Overlay{
		overlayType: OverlaySummary,
		title:       title,
		message:     body,
		cursor:      1, // default to Initialize
		active:      true,
	}
}

// NewChoiceOverlay creates a list-of-choices dialog.
func NewChoiceOverlay(title string, choices []string) Overlay {
	return Overlay{
		overlayType: OverlayChoice,
		title:       title,
		choices:     choices,
		cursor:      0,
		active:      true,
	}
}

// Active returns whether the overlay is currently shown.
func (o Overlay) Active() bool {
	return o.active
}

// Update handles key messages for the overlay.
func (o Overlay) Update(msg tea.Msg) (Overlay, tea.Cmd) {
	if !o.active {
		return o, nil
	}

	switch o.overlayType {
	case OverlayConfirm:
		return o.updateConfirm(msg)
	case OverlayTextInput:
		return o.updateTextInput(msg)
	case OverlaySummary:
		return o.updateSummary(msg)
	case OverlayChoice:
		return o.updateChoice(msg)
	}
	return o, nil
}

func (o Overlay) updateConfirm(msg tea.Msg) (Overlay, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			o.active = false
			return o, func() tea.Msg {
				return OverlayCloseMsg{Confirmed: false}
			}
		case "tab", "left", "right", "h", "l":
			o.cursor = 1 - o.cursor // toggle between 0 and 1
		case "enter":
			o.active = false
			return o, func() tea.Msg {
				return OverlayCloseMsg{Confirmed: o.cursor == 1}
			}
		}
	}
	return o, nil
}

func (o Overlay) updateTextInput(msg tea.Msg) (Overlay, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			o.active = false
			return o, func() tea.Msg {
				return OverlayCloseMsg{Confirmed: false}
			}
		case "enter":
			value := o.input.Value()
			if value == "" {
				return o, nil // don't submit empty
			}
			o.active = false
			return o, func() tea.Msg {
				return OverlayCloseMsg{Result: value, Confirmed: true}
			}
		}
	}

	// Delegate other keys to the text input.
	var cmd tea.Cmd
	o.input, cmd = o.input.Update(msg)
	return o, cmd
}

func (o Overlay) updateSummary(msg tea.Msg) (Overlay, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			o.active = false
			return o, func() tea.Msg {
				return OverlayCloseMsg{Confirmed: false}
			}
		case "tab", "left", "right", "h", "l":
			o.cursor = 1 - o.cursor
		case "enter":
			o.active = false
			return o, func() tea.Msg {
				return OverlayCloseMsg{Confirmed: o.cursor == 1}
			}
		}
	}
	return o, nil
}

func (o Overlay) updateChoice(msg tea.Msg) (Overlay, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			o.active = false
			return o, func() tea.Msg {
				return OverlayCloseMsg{Confirmed: false}
			}
		case "up", "k":
			if o.cursor > 0 {
				o.cursor--
			}
		case "down", "j":
			if o.cursor < len(o.choices)-1 {
				o.cursor++
			}
		case "enter":
			o.active = false
			result := ""
			if o.cursor >= 0 && o.cursor < len(o.choices) {
				result = o.choices[o.cursor]
			}
			return o, func() tea.Msg {
				return OverlayCloseMsg{Result: result, Confirmed: true}
			}
		}
	}
	return o, nil
}

// View renders the overlay box. It does not composite over a background;
// that is the caller's responsibility using Composite().
func (o Overlay) View() string {
	if !o.active {
		return ""
	}

	var content string
	switch o.overlayType {
	case OverlayConfirm:
		content = o.viewConfirm()
	case OverlayTextInput:
		content = o.viewTextInput()
	case OverlaySummary:
		content = o.viewSummary()
	case OverlayChoice:
		content = o.viewChoice()
	}

	return OverlayStyle.Render(content)
}

func (o Overlay) viewConfirm() string {
	var b strings.Builder
	b.WriteString(OverlayTitleStyle.Render(o.title))
	b.WriteString("\n\n")
	b.WriteString(o.message)
	b.WriteString("\n\n")
	b.WriteString(o.renderButtons("Cancel", "OK"))
	return b.String()
}

func (o Overlay) viewTextInput() string {
	var b strings.Builder
	b.WriteString(OverlayTitleStyle.Render(o.title))
	b.WriteString("\n\n")
	b.WriteString(o.input.View())
	b.WriteString("\n\n")
	b.WriteString(lipgloss.NewStyle().Foreground(colorOverlay0).Render("Enter: submit  Esc: cancel"))
	return b.String()
}

func (o Overlay) viewSummary() string {
	var b strings.Builder
	b.WriteString(OverlayTitleStyle.Render(o.title))
	b.WriteString("\n\n")
	b.WriteString(o.message)
	b.WriteString("\n\n")
	b.WriteString(o.renderButtons("Cancel", "Initialize"))
	return b.String()
}

func (o Overlay) viewChoice() string {
	var b strings.Builder
	b.WriteString(OverlayTitleStyle.Render(o.title))
	b.WriteString("\n\n")
	for i, choice := range o.choices {
		if i == o.cursor {
			b.WriteString(OverlayChoiceCursorStyle.Render("> " + choice))
		} else {
			b.WriteString("  " + choice)
		}
		b.WriteString("\n")
	}
	return b.String()
}

// renderButtons draws two side-by-side buttons with the cursor on one.
func (o Overlay) renderButtons(cancel, ok string) string {
	var cancelBtn, okBtn string
	if o.cursor == 0 {
		cancelBtn = OverlayButtonActiveStyle.Render(cancel)
		okBtn = OverlayButtonInactiveStyle.Render(ok)
	} else {
		cancelBtn = OverlayButtonInactiveStyle.Render(cancel)
		okBtn = OverlayButtonActiveStyle.Render(ok)
	}
	return cancelBtn + "  " + okBtn
}

// Composite places the overlay box centered on top of the background string.
// The background is expected to be a fully rendered terminal frame.
func Composite(background string, overlay string, totalWidth, totalHeight int) string {
	if overlay == "" {
		return background
	}

	bgLines := strings.Split(background, "\n")

	// Pad background to fill the screen height.
	for len(bgLines) < totalHeight {
		bgLines = append(bgLines, "")
	}

	overlayLines := strings.Split(overlay, "\n")
	overlayHeight := len(overlayLines)
	overlayWidth := 0
	for _, line := range overlayLines {
		w := ansi.StringWidth(line)
		if w > overlayWidth {
			overlayWidth = w
		}
	}

	// Calculate centering offsets.
	startRow := (totalHeight - overlayHeight) / 2
	if startRow < 0 {
		startRow = 0
	}
	startCol := (totalWidth - overlayWidth) / 2
	if startCol < 0 {
		startCol = 0
	}

	// Overlay each line onto the background.
	for i, overlayLine := range overlayLines {
		row := startRow + i
		if row >= len(bgLines) {
			break
		}

		bgLine := bgLines[row]
		bgRunes := []rune(bgLine)

		// Build the composited line: background left + overlay + background right.
		leftPad := ""
		if startCol > 0 {
			if startCol <= len(bgRunes) {
				leftPad = string(bgRunes[:startCol])
			} else {
				leftPad = string(bgRunes) + strings.Repeat(" ", startCol-len(bgRunes))
			}
		}

		overlayEnd := startCol + ansi.StringWidth(overlayLine)
		rightPad := ""
		if overlayEnd < len(bgRunes) {
			rightPad = string(bgRunes[overlayEnd:])
		}

		bgLines[row] = leftPad + overlayLine + rightPad
	}

	return strings.Join(bgLines[:totalHeight], "\n")
}

// SetWidth sets the overlay box width hint. Currently used for rendering hints.
func (o *Overlay) SetWidth(w int) {
	o.width = w
	if o.overlayType == OverlayTextInput {
		inputWidth := w - 6 // account for overlay padding and border
		if inputWidth < 20 {
			inputWidth = 20
		}
		o.input.Width = inputWidth
	}
}

// OverlayMinWidth returns a reasonable minimum width for the overlay content.
func OverlayMinWidth() int {
	return 40
}

// OverlayMaxWidth returns a reasonable maximum width for the overlay content.
func OverlayMaxWidth(termWidth int) int {
	w := termWidth * 2 / 3
	if w < 40 {
		w = 40
	}
	if w > 60 {
		w = 60
	}
	return w
}

// FormatSummaryBody builds the summary body text from init stats.
func FormatSummaryBody(stats map[string]int, profiles []string) string {
	var b strings.Builder
	parts := []struct {
		key   string
		label string
	}{
		{"plugins", "plugins"},
		{"settings", "settings"},
		{"claudemd", "CLAUDE.md sections"},
		{"permissions", "permissions"},
		{"mcp", "MCP servers"},
		{"keybindings", "keybindings"},
		{"hooks", "hooks"},
	}

	b.WriteString("Base: ")
	first := true
	for _, p := range parts {
		if count, ok := stats[p.key]; ok && count > 0 {
			if !first {
				b.WriteString(", ")
			}
			b.WriteString(fmt.Sprintf("%d %s", count, p.label))
			first = false
		}
	}
	b.WriteString("\n")

	if len(profiles) > 0 {
		b.WriteString("\nProfiles: ")
		b.WriteString(strings.Join(profiles, ", "))
		b.WriteString("\n")
	}

	return b.String()
}
