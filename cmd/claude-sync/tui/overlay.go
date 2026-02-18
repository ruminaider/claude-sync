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
	OverlayProfileList                   // Batch profile name inputs with Done button
)

// Overlay renders a centered modal box on top of existing content.
type Overlay struct {
	overlayType OverlayType
	title       string
	message     string   // body text (for Confirm, Summary)
	choices     []string // choice list (for Choice)
	cursor      int      // selected choice index (Choice), or button index (Confirm/Summary: 0=Cancel, 1=OK)
	input       textinput.Model
	inputs      []textinput.Model // profile name inputs (for ProfileList)
	activeLine  int               // focused input index; len(inputs) = Done button
	width       int               // overlay box width
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

// NewProfileListOverlay creates a batch profile name input overlay.
func NewProfileListOverlay() Overlay {
	ti := textinput.New()
	ti.Placeholder = "e.g. work"
	ti.Focus()
	ti.CharLimit = 64
	ti.Width = 30
	return Overlay{
		overlayType: OverlayProfileList,
		title:       "Profile names",
		message:     "Base config is created automatically.\nAdd your profiles:",
		inputs:      []textinput.Model{ti},
		activeLine:  0,
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
	case OverlayProfileList:
		return o.updateProfileList(msg)
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

func (o Overlay) updateProfileList(msg tea.Msg) (Overlay, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			o.active = false
			return o, func() tea.Msg {
				return OverlayCloseMsg{Confirmed: false}
			}

		case "up":
			if o.activeLine > 0 {
				// Blur current input (if on an input row).
				if o.activeLine < len(o.inputs) {
					o.inputs[o.activeLine].Blur()
				}
				o.activeLine--
				o.inputs[o.activeLine].Focus()
			}
			return o, nil

		case "down":
			if o.activeLine < len(o.inputs) {
				// Moving down from an input row.
				o.inputs[o.activeLine].Blur()
				o.activeLine++
				if o.activeLine < len(o.inputs) {
					o.inputs[o.activeLine].Focus()
				}
			}
			return o, nil

		case "enter":
			if o.activeLine == len(o.inputs) {
				// On Done button — validate and submit.
				var valid []string
				seen := make(map[string]bool)
				for _, inp := range o.inputs {
					name := strings.TrimSpace(strings.ToLower(inp.Value()))
					if name == "" || name == "base" || seen[name] {
						continue
					}
					seen[name] = true
					valid = append(valid, name)
				}
				if len(valid) == 0 {
					return o, nil // stay on overlay
				}
				o.active = false
				return o, func() tea.Msg {
					return OverlayCloseMsg{Results: valid, Confirmed: true}
				}
			}
			// On an input row with non-empty value — insert new row below.
			if strings.TrimSpace(o.inputs[o.activeLine].Value()) != "" {
				o.inputs[o.activeLine].Blur()
				newInput := textinput.New()
				newInput.Placeholder = "e.g. personal"
				newInput.CharLimit = 64
				newInput.Width = o.inputs[o.activeLine].Width
				newInput.Focus()
				// Insert after activeLine.
				idx := o.activeLine + 1
				o.inputs = append(o.inputs, textinput.Model{})
				copy(o.inputs[idx+1:], o.inputs[idx:])
				o.inputs[idx] = newInput
				o.activeLine = idx
			}
			return o, nil

		case "backspace":
			if o.activeLine < len(o.inputs) && o.inputs[o.activeLine].Value() == "" && len(o.inputs) > 1 {
				// Remove empty input row.
				o.inputs = append(o.inputs[:o.activeLine], o.inputs[o.activeLine+1:]...)
				if o.activeLine >= len(o.inputs) {
					o.activeLine = len(o.inputs) - 1
				}
				o.inputs[o.activeLine].Focus()
				return o, nil
			}
		}
	}

	// Delegate to the active input for text entry.
	if o.activeLine < len(o.inputs) {
		var cmd tea.Cmd
		o.inputs[o.activeLine], cmd = o.inputs[o.activeLine].Update(msg)
		return o, cmd
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
	case OverlayProfileList:
		content = o.viewProfileList()
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
	if o.message != "" {
		b.WriteString(lipgloss.NewStyle().Foreground(colorOverlay0).Render(o.message))
		b.WriteString("\n\n")
	}
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

func (o Overlay) viewProfileList() string {
	var b strings.Builder
	b.WriteString(OverlayTitleStyle.Render(o.title))
	b.WriteString("\n\n")
	b.WriteString(lipgloss.NewStyle().Foreground(colorOverlay0).Render(o.message))
	b.WriteString("\n\n")

	for i, inp := range o.inputs {
		if i == o.activeLine {
			b.WriteString("> ")
		} else {
			b.WriteString("  ")
		}
		b.WriteString(inp.View())
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString(lipgloss.NewStyle().Foreground(colorOverlay0).Render("Enter: add  ⌫ empty: remove"))
	b.WriteString("\n\n")

	// Done button.
	var doneBtn string
	if o.activeLine == len(o.inputs) {
		doneBtn = OverlayButtonActiveStyle.Render("Done")
	} else {
		doneBtn = OverlayButtonInactiveStyle.Render("Done")
	}
	// Center the button.
	b.WriteString("        " + doneBtn)

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

	// Blank out the entire background so nothing bleeds through.
	for i := range bgLines {
		bgLines[i] = strings.Repeat(" ", totalWidth)
	}

	// Place the overlay centered on the blank backdrop.
	for i, overlayLine := range overlayLines {
		row := startRow + i
		if row >= len(bgLines) {
			break
		}

		leftPad := strings.Repeat(" ", startCol)
		bgLines[row] = leftPad + overlayLine
	}

	return strings.Join(bgLines[:totalHeight], "\n")
}

// SetWidth sets the overlay box width hint. Currently used for rendering hints.
func (o *Overlay) SetWidth(w int) {
	o.width = w
	inputWidth := w - 6 // account for overlay padding and border
	if inputWidth < 20 {
		inputWidth = 20
	}
	switch o.overlayType {
	case OverlayTextInput:
		o.input.Width = inputWidth
	case OverlayProfileList:
		for i := range o.inputs {
			o.inputs[i].Width = inputWidth
		}
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
