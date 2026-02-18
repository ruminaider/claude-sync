package tui

import (
	"strings"
	"testing"

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
