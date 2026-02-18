package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestComposite(t *testing.T) {
	bg := "AAAA\nBBBB\nCCCC\nDDDD"
	overlay := "XX\nXX"
	result := Composite(bg, overlay, 4, 4)
	lines := strings.Split(result, "\n")
	require.Equal(t, 4, len(lines))

	// Overlay should be centered vertically (rows 1-2 of 0-3)
	// and horizontally (col 1 of 0-3)
	// All background content should be blanked out
	assert.Equal(t, "    ", lines[0])
	// Rows 1-2 should contain overlay centered
	assert.Contains(t, lines[1], "XX")
	assert.Contains(t, lines[2], "XX")
	assert.Equal(t, "    ", lines[3])
}

func TestCompositeEmpty(t *testing.T) {
	bg := "hello"
	result := Composite(bg, "", 5, 1)
	assert.Equal(t, bg, result)
}

func TestComposite_SingleLine(t *testing.T) {
	bg := "AAAA\nBBBB\nCCCC"
	overlay := "X"
	result := Composite(bg, overlay, 4, 3)
	lines := strings.Split(result, "\n")
	require.Equal(t, 3, len(lines))
	// Overlay should be on the middle row (index 1)
	assert.Contains(t, lines[1], "X")
}

func TestComposite_OversizedOverlay(t *testing.T) {
	bg := "A\nB"
	overlay := "XXXX\nXXXX\nXXXX\nXXXX"
	// Overlay bigger than background - should not panic
	result := Composite(bg, overlay, 2, 2)
	assert.NotEmpty(t, result)
}

func TestFormatSummaryBody(t *testing.T) {
	stats := map[string]int{
		"plugins":      5,
		"settings":     2,
		"claudemd":     3,
		"permissions":  1,
		"mcp":          0,
		"keybindings":  0,
		"hooks":        0,
	}
	body := FormatSummaryBody(stats, []string{"work"})
	assert.Contains(t, body, "5 plugins")
	assert.Contains(t, body, "2 settings")
	assert.Contains(t, body, "3 CLAUDE.md sections")
	assert.Contains(t, body, "1 permissions")
	assert.NotContains(t, body, "0 MCP") // zero counts should not appear
	assert.Contains(t, body, "work")
	assert.Contains(t, body, "Profiles:")
}

func TestFormatSummaryBody_NoProfiles(t *testing.T) {
	stats := map[string]int{"plugins": 3}
	body := FormatSummaryBody(stats, nil)
	assert.Contains(t, body, "3 plugins")
	assert.NotContains(t, body, "Profiles:")
}

func TestFormatSummaryBody_MultipleProfiles(t *testing.T) {
	stats := map[string]int{"plugins": 1}
	body := FormatSummaryBody(stats, []string{"work", "personal"})
	assert.Contains(t, body, "work, personal")
}

func TestFormatSummaryBody_AllZero(t *testing.T) {
	stats := map[string]int{
		"plugins":  0,
		"settings": 0,
	}
	body := FormatSummaryBody(stats, nil)
	// Should have "Base: " but no items listed after it
	assert.Contains(t, body, "Base: ")
}

func TestOverlayMinMaxWidth(t *testing.T) {
	assert.Equal(t, 40, OverlayMinWidth())

	// Small terminal
	assert.Equal(t, 40, OverlayMaxWidth(50))

	// Medium terminal
	assert.Equal(t, 53, OverlayMaxWidth(80))

	// Large terminal
	assert.Equal(t, 60, OverlayMaxWidth(120))
}

func TestNewConfirmOverlay(t *testing.T) {
	o := NewConfirmOverlay("Delete?", "Are you sure?")
	assert.True(t, o.Active())
	assert.Equal(t, OverlayConfirm, o.overlayType)
	assert.Equal(t, "Delete?", o.title)
	assert.Equal(t, "Are you sure?", o.message)
	assert.Equal(t, 1, o.cursor) // defaults to OK
}

func TestNewTextInputOverlay(t *testing.T) {
	o := NewTextInputOverlay("Name", "placeholder")
	assert.True(t, o.Active())
	assert.Equal(t, OverlayTextInput, o.overlayType)
	assert.Equal(t, "Name", o.title)
}

func TestNewSummaryOverlay(t *testing.T) {
	o := NewSummaryOverlay("Summary", "body text")
	assert.True(t, o.Active())
	assert.Equal(t, OverlaySummary, o.overlayType)
	assert.Equal(t, 1, o.cursor) // defaults to Initialize
}

func TestNewChoiceOverlay(t *testing.T) {
	o := NewChoiceOverlay("Pick one", []string{"A", "B", "C"})
	assert.True(t, o.Active())
	assert.Equal(t, OverlayChoice, o.overlayType)
	assert.Equal(t, 3, len(o.choices))
	assert.Equal(t, 0, o.cursor)
}

func TestOverlayInactive(t *testing.T) {
	o := Overlay{}
	assert.False(t, o.Active())
	assert.Equal(t, "", o.View())
}

func TestOverlaySetWidth(t *testing.T) {
	o := NewTextInputOverlay("Test", "ph")
	o.SetWidth(50)
	assert.Equal(t, 50, o.width)
	// Input width should be adjusted (50 - 6 = 44)
	assert.Equal(t, 44, o.input.Width)
}

func TestOverlaySetWidth_Small(t *testing.T) {
	o := NewTextInputOverlay("Test", "ph")
	o.SetWidth(10)
	// Input width should be at least 20
	assert.GreaterOrEqual(t, o.input.Width, 20)
}

// --- ProfileList overlay tests ---

func TestNewProfileListOverlay(t *testing.T) {
	o := NewProfileListOverlay()
	assert.True(t, o.Active())
	assert.Equal(t, OverlayProfileList, o.overlayType)
	assert.Equal(t, "Profile names", o.title)
	require.Len(t, o.inputs, 1)
	assert.Equal(t, 0, o.activeLine)
	assert.True(t, o.inputs[0].Focused())
}

func TestProfileListOverlay_SetWidth(t *testing.T) {
	o := NewProfileListOverlay()
	// Add a second input for testing.
	o.inputs = append(o.inputs, o.inputs[0])
	o.SetWidth(50)
	assert.Equal(t, 50, o.width)
	for _, inp := range o.inputs {
		assert.Equal(t, 44, inp.Width)
	}
}

func TestProfileListOverlay_AddRow(t *testing.T) {
	o := NewProfileListOverlay()

	// Type "work" into the first input.
	o.inputs[0].SetValue("work")

	// Press Enter to add a new row.
	o, _ = o.Update(tea.KeyMsg{Type: tea.KeyEnter})

	require.Len(t, o.inputs, 2)
	assert.Equal(t, 1, o.activeLine, "should focus new row")
	assert.True(t, o.inputs[1].Focused())
	assert.Equal(t, "work", o.inputs[0].Value())
}

func TestProfileListOverlay_AddRow_EmptyField(t *testing.T) {
	o := NewProfileListOverlay()

	// With empty field, Enter should not add a row.
	o, _ = o.Update(tea.KeyMsg{Type: tea.KeyEnter})

	assert.Len(t, o.inputs, 1, "should not add row when field is empty")
	assert.Equal(t, 0, o.activeLine)
}

func TestProfileListOverlay_RemoveRow(t *testing.T) {
	o := NewProfileListOverlay()

	// Add a second row: type "work", enter, now on row 1.
	o.inputs[0].SetValue("work")
	o, _ = o.Update(tea.KeyMsg{Type: tea.KeyEnter})
	require.Len(t, o.inputs, 2)

	// Row 1 is empty and focused — Backspace should remove it.
	o, _ = o.Update(tea.KeyMsg{Type: tea.KeyBackspace})

	assert.Len(t, o.inputs, 1)
	assert.Equal(t, 0, o.activeLine)
	assert.Equal(t, "work", o.inputs[0].Value())
}

func TestProfileListOverlay_RemoveRow_MinOne(t *testing.T) {
	o := NewProfileListOverlay()

	// Only one row, empty — Backspace should NOT remove it.
	o, _ = o.Update(tea.KeyMsg{Type: tea.KeyBackspace})

	assert.Len(t, o.inputs, 1, "should not remove last row")
}

func TestProfileListOverlay_Submit(t *testing.T) {
	o := NewProfileListOverlay()

	// Type "work", add row, type "personal".
	o.inputs[0].SetValue("work")
	o, _ = o.Update(tea.KeyMsg{Type: tea.KeyEnter})
	o.inputs[1].SetValue("personal")

	// Navigate to Done (down past last input).
	o, _ = o.Update(tea.KeyMsg{Type: tea.KeyDown})
	assert.Equal(t, len(o.inputs), o.activeLine, "should be on Done")

	// Press Enter on Done.
	o, cmd := o.Update(tea.KeyMsg{Type: tea.KeyEnter})
	assert.False(t, o.Active())

	closeMsg := extractOverlayClose(cmd)
	require.NotNil(t, closeMsg)
	assert.True(t, closeMsg.Confirmed)
	assert.Equal(t, []string{"work", "personal"}, closeMsg.Results)
}

func TestProfileListOverlay_Submit_FiltersDuplicates(t *testing.T) {
	o := NewProfileListOverlay()

	o.inputs[0].SetValue("work")
	o, _ = o.Update(tea.KeyMsg{Type: tea.KeyEnter})
	o.inputs[1].SetValue("WORK") // duplicate (case insensitive)

	// Navigate to Done.
	o, _ = o.Update(tea.KeyMsg{Type: tea.KeyDown})
	o, cmd := o.Update(tea.KeyMsg{Type: tea.KeyEnter})

	closeMsg := extractOverlayClose(cmd)
	require.NotNil(t, closeMsg)
	assert.Equal(t, []string{"work"}, closeMsg.Results, "duplicates should be filtered")
}

func TestProfileListOverlay_Submit_FiltersBase(t *testing.T) {
	o := NewProfileListOverlay()

	o.inputs[0].SetValue("base")
	o, _ = o.Update(tea.KeyMsg{Type: tea.KeyEnter})
	o.inputs[1].SetValue("work")

	// Navigate to Done.
	o, _ = o.Update(tea.KeyMsg{Type: tea.KeyDown})
	o, cmd := o.Update(tea.KeyMsg{Type: tea.KeyEnter})

	closeMsg := extractOverlayClose(cmd)
	require.NotNil(t, closeMsg)
	assert.Equal(t, []string{"work"}, closeMsg.Results, "\"base\" should be filtered")
}

func TestProfileListOverlay_Submit_AllEmpty(t *testing.T) {
	o := NewProfileListOverlay()

	// Navigate to Done with no input.
	o, _ = o.Update(tea.KeyMsg{Type: tea.KeyDown})
	o, _ = o.Update(tea.KeyMsg{Type: tea.KeyEnter})

	assert.True(t, o.Active(), "should stay on overlay when no valid names")
}

func TestProfileListOverlay_Navigation(t *testing.T) {
	o := NewProfileListOverlay()

	// Add a second row.
	o.inputs[0].SetValue("work")
	o, _ = o.Update(tea.KeyMsg{Type: tea.KeyEnter})
	require.Len(t, o.inputs, 2)
	assert.Equal(t, 1, o.activeLine)

	// Down → Done button.
	o, _ = o.Update(tea.KeyMsg{Type: tea.KeyDown})
	assert.Equal(t, 2, o.activeLine) // len(inputs) = Done

	// Down again → stay on Done.
	o, _ = o.Update(tea.KeyMsg{Type: tea.KeyDown})
	assert.Equal(t, 2, o.activeLine)

	// Up → back to row 1.
	o, _ = o.Update(tea.KeyMsg{Type: tea.KeyUp})
	assert.Equal(t, 1, o.activeLine)
	assert.True(t, o.inputs[1].Focused())

	// Up → row 0.
	o, _ = o.Update(tea.KeyMsg{Type: tea.KeyUp})
	assert.Equal(t, 0, o.activeLine)
	assert.True(t, o.inputs[0].Focused())

	// Up again → stay on row 0.
	o, _ = o.Update(tea.KeyMsg{Type: tea.KeyUp})
	assert.Equal(t, 0, o.activeLine)
}

func TestProfileListOverlay_EscCancels(t *testing.T) {
	o := NewProfileListOverlay()
	o.inputs[0].SetValue("work")

	o, cmd := o.Update(tea.KeyMsg{Type: tea.KeyEsc})
	assert.False(t, o.Active())

	closeMsg := extractOverlayClose(cmd)
	require.NotNil(t, closeMsg)
	assert.False(t, closeMsg.Confirmed)
}
