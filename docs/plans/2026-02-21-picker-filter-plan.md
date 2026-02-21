# Picker Filter Bar Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add an always-visible type-to-filter text field to every TUI picker section, enabling real-time item narrowing via case-insensitive substring match on Display + Tag.

**Architecture:** Add `filterText` and `filterView []int` (cached visible indices) to the Picker struct. When `filterView` is nil, no filter is active and all items render as before. When non-nil, `filterView` drives rendering, cursor movement, and scroll. Keyboard handling changes: printable chars go to filter input, `j/k/a/n/h/l` are sacrificed, `Ctrl+A/Ctrl+N` replace bulk select.

**Tech Stack:** Go, bubbletea, lipgloss (existing)

---

### Task 1: Add filter infrastructure to Picker

**Files:**
- Modify: `cmd/claude-sync/tui/picker.go`
- Test: `cmd/claude-sync/tui/picker_test.go` (new file)

**Step 1: Write the failing tests**

Create `cmd/claude-sync/tui/picker_test.go`:

```go
package tui

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func testItems() []PickerItem {
	return []PickerItem{
		{Display: "Upstream (2)", IsHeader: true},
		{Description: "Marketplace plugins"},
		{Key: "a@m", Display: "alpha-plugin", Selected: true},
		{Key: "b@m", Display: "beta-plugin", Selected: true, Tag: "[cmd] beads"},
		{Display: "Auto-forked (1)", IsHeader: true},
		{Description: "Local plugins"},
		{Key: "c@local", Display: "charlie", Selected: true, Tag: "[skill]"},
	}
}

func TestRefilter_NoFilter(t *testing.T) {
	p := NewPicker(testItems())
	p.refilter()
	assert.Nil(t, p.filterView, "no filter text should produce nil filterView")
}

func TestRefilter_MatchesByDisplay(t *testing.T) {
	p := NewPicker(testItems())
	p.filterText = "alpha"
	p.refilter()
	// Should show: Upstream header, description, alpha-plugin
	assert.NotNil(t, p.filterView)
	assert.Equal(t, 3, len(p.filterView)) // header + desc + alpha
}

func TestRefilter_MatchesByTag(t *testing.T) {
	p := NewPicker(testItems())
	p.filterText = "beads"
	p.refilter()
	// Should show: Upstream header, description, beta-plugin (matched via tag)
	assert.Equal(t, 3, len(p.filterView))
}

func TestRefilter_CaseInsensitive(t *testing.T) {
	p := NewPicker(testItems())
	p.filterText = "ALPHA"
	p.refilter()
	assert.Equal(t, 3, len(p.filterView))
}

func TestRefilter_NoMatches(t *testing.T) {
	p := NewPicker(testItems())
	p.filterText = "zzz"
	p.refilter()
	assert.NotNil(t, p.filterView)
	assert.Equal(t, 0, len(p.filterView))
}

func TestRefilter_HidesEmptyHeaders(t *testing.T) {
	p := NewPicker(testItems())
	p.filterText = "charlie"
	p.refilter()
	// Should show: Auto-forked header, description, charlie
	// NOT Upstream header (no matching children)
	assert.Equal(t, 3, len(p.filterView))
	assert.Equal(t, 4, p.filterView[0]) // Auto-forked header at index 4
}

func TestRefilter_MatchesSkill(t *testing.T) {
	p := NewPicker(testItems())
	p.filterText = "skill"
	p.refilter()
	// [skill] tag on charlie matches
	assert.Equal(t, 3, len(p.filterView)) // Auto-forked header + desc + charlie
}

func TestVisibleSelectableCount(t *testing.T) {
	p := NewPicker(testItems())
	assert.Equal(t, 3, p.visibleSelectableCount()) // 3 selectable items

	p.filterText = "alpha"
	p.refilter()
	assert.Equal(t, 1, p.visibleSelectableCount())
}
```

**Step 2: Run tests to verify they fail**

Run: `go test ./cmd/claude-sync/tui/ -run TestRefilter -v`
Expected: FAIL (methods don't exist yet)

**Step 3: Implement filter infrastructure**

In `picker.go`, add fields to the `Picker` struct (after `previewScroll`):

```go
// Filter mode: type-to-filter narrows the visible item list.
filterText string
filterView []int // nil = no filter; otherwise, indices into items to render
```

Add methods after the `AllKeys()` method:

```go
// refilter recomputes filterView based on current filterText.
// When filterText is empty, filterView is set to nil (show all).
func (p *Picker) refilter() {
	if p.filterText == "" {
		p.filterView = nil
		return
	}

	needle := strings.ToLower(p.filterText)

	// First pass: determine which selectable items match.
	matches := make([]bool, len(p.items))
	for i, it := range p.items {
		if it.IsHeader || it.Description != "" {
			continue
		}
		haystack := strings.ToLower(it.Display + " " + it.Tag)
		if strings.Contains(haystack, needle) {
			matches[i] = true
		}
	}

	// Second pass: build visible indices, including headers/descriptions
	// only if they have at least one matching child.
	var vis []int
	for i, it := range p.items {
		if matches[i] {
			vis = append(vis, i)
			continue
		}
		if it.IsHeader {
			// Include header if any child before the next header matches.
			for j := i + 1; j < len(p.items); j++ {
				if p.items[j].IsHeader {
					break
				}
				if matches[j] {
					vis = append(vis, i)
					break
				}
			}
		} else if it.Description != "" {
			// Include description if its preceding header was included.
			if len(vis) > 0 && vis[len(vis)-1] == i-1 {
				vis = append(vis, i)
			}
		}
	}

	p.filterView = vis
}

// isFilteredOut returns true if the item at index i is hidden by the active filter.
func (p *Picker) isFilteredOut(i int) bool {
	if p.filterView == nil {
		return false
	}
	for _, vi := range p.filterView {
		if vi == i {
			return false
		}
	}
	return true
}

// visibleSelectableCount returns the number of selectable items currently visible.
func (p Picker) visibleSelectableCount() int {
	n := 0
	for i, it := range p.items {
		if isSelectableItem(it) && !p.isFilteredOut(i) {
			n++
		}
	}
	return n
}
```

Update `isSkippable` to also skip filtered-out items:

```go
func (p *Picker) isSkippable(i int) bool {
	if i < 0 || i >= len(p.items) {
		return false
	}
	if p.items[i].IsHeader || p.items[i].Description != "" {
		return true
	}
	if p.isFilteredOut(i) {
		return true
	}
	return false
}
```

Update `SetItems` to clear filter when items are replaced:

```go
func (p *Picker) SetItems(items []PickerItem) {
	p.items = items
	p.cursor = 0
	p.offset = 0
	p.filterText = ""
	p.filterView = nil
	// ... rest unchanged
}
```

**Step 4: Run tests to verify they pass**

Run: `go test ./cmd/claude-sync/tui/ -run TestRefilter -v`
Expected: PASS

Run: `go test ./cmd/claude-sync/tui/ -run TestVisibleSelectable -v`
Expected: PASS

**Step 5: Run full test suite to check for regressions**

Run: `go test ./cmd/claude-sync/tui/ -v`
Expected: All existing tests PASS (filter defaults to inactive)

**Step 6: Commit**

```bash
git add cmd/claude-sync/tui/picker.go cmd/claude-sync/tui/picker_test.go
git commit -m "feat: add filter infrastructure to picker"
```

---

### Task 2: Update keyboard handling

**Files:**
- Modify: `cmd/claude-sync/tui/picker.go`
- Test: `cmd/claude-sync/tui/picker_test.go`

**Step 1: Write the failing tests**

Add to `picker_test.go`:

```go
func TestFilterKeyInput(t *testing.T) {
	p := NewPicker(testItems())
	p.focused = true

	// Type "a" - should go to filter
	p, _ = p.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	assert.Equal(t, "a", p.filterText)

	// Type "l" - should append
	p, _ = p.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
	assert.Equal(t, "al", p.filterText)

	// Backspace - should delete last char
	p, _ = p.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	assert.Equal(t, "a", p.filterText)

	// Backspace again - empty
	p, _ = p.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	assert.Equal(t, "", p.filterText)
}

func TestEscClearsFilter(t *testing.T) {
	p := NewPicker(testItems())
	p.focused = true
	p.filterText = "alpha"
	p.refilter()

	// Esc should clear filter text (not emit focus change)
	p, cmd := p.Update(tea.KeyMsg{Type: tea.KeyEscape})
	assert.Equal(t, "", p.filterText)
	assert.Nil(t, p.filterView, "filterView should be nil after clearing")
	assert.Nil(t, cmd, "should not emit focus change when clearing filter")
}

func TestEscEmitsFocusWhenFilterEmpty(t *testing.T) {
	p := NewPicker(testItems())
	p.focused = true
	p.filterText = ""

	// Esc with empty filter should emit focus change
	_, cmd := p.Update(tea.KeyMsg{Type: tea.KeyEscape})
	assert.NotNil(t, cmd, "should emit focus change when filter is empty")
}

func TestCtrlASelectsVisibleOnly(t *testing.T) {
	p := NewPicker(testItems())
	p.focused = true
	// Deselect all first
	for i := range p.items {
		p.items[i].Selected = false
	}

	// Filter to "alpha"
	p.filterText = "alpha"
	p.refilter()

	// Ctrl+A should only select visible items
	p, _ = p.Update(tea.KeyMsg{Type: tea.KeyCtrlA})
	assert.True(t, p.items[2].Selected, "alpha should be selected")
	assert.False(t, p.items[3].Selected, "beta should NOT be selected")
	assert.False(t, p.items[6].Selected, "charlie should NOT be selected")
}

func TestCtrlNDeselectsVisibleOnly(t *testing.T) {
	p := NewPicker(testItems())
	p.focused = true
	// All start selected

	// Filter to "alpha"
	p.filterText = "alpha"
	p.refilter()

	// Ctrl+N should only deselect visible items
	p, _ = p.Update(tea.KeyMsg{Type: tea.KeyCtrlN})
	assert.False(t, p.items[2].Selected, "alpha should be deselected")
	assert.True(t, p.items[3].Selected, "beta should still be selected")
	assert.True(t, p.items[6].Selected, "charlie should still be selected")
}

func TestArrowKeysStillNavigate(t *testing.T) {
	p := NewPicker(testItems())
	p.focused = true

	startCursor := p.cursor
	p, _ = p.Update(tea.KeyMsg{Type: tea.KeyDown})
	assert.NotEqual(t, startCursor, p.cursor, "down arrow should move cursor")
}

func TestFilterResetsOnSetItems(t *testing.T) {
	p := NewPicker(testItems())
	p.filterText = "alpha"
	p.refilter()

	p.SetItems(testItems())
	assert.Equal(t, "", p.filterText)
	assert.Nil(t, p.filterView)
}
```

**Step 2: Run tests to verify they fail**

Run: `go test ./cmd/claude-sync/tui/ -run "TestFilter|TestEsc|TestCtrl|TestArrow" -v`
Expected: FAIL

**Step 3: Implement keyboard handling changes**

Replace the `Update` method's non-preview key handling block (the `switch msg.String()` after preview handling). The full replacement:

```go
// In Update(), replace the switch block after preview handling:

switch msg.String() {
case "up":
	p.moveCursor(-1)
case "down":
	p.moveCursor(+1)
case " ":
	if !(p.hasSearchAction && p.cursor == len(p.items)) {
		p.toggleCurrent()
	}
case "enter":
	if p.hasSearchAction && p.cursor == len(p.items) {
		return p, func() tea.Msg {
			return SearchRequestMsg{}
		}
	}
	p.toggleCurrent()
case "right":
	if p.filterText == "" && p.hasPreview && p.cursor >= 0 && p.cursor < len(p.items) {
		key := p.items[p.cursor].Key
		if _, ok := p.previewContent[key]; ok {
			p.previewActive = true
			p.previewScroll = 0
			return p, nil
		}
	}
case "left":
	if p.filterText == "" {
		return p, func() tea.Msg {
			return FocusChangeMsg{Zone: FocusSidebar}
		}
	}
case "esc":
	if p.filterText != "" {
		p.filterText = ""
		p.refilter()
		p.resetCursorToFirstVisible()
		return p, nil
	}
	return p, func() tea.Msg {
		return FocusChangeMsg{Zone: FocusSidebar}
	}
case "ctrl+a":
	p.doSelectAll()
case "ctrl+n":
	p.doSelectNone()
case "backspace":
	if len(p.filterText) > 0 {
		p.filterText = p.filterText[:len(p.filterText)-1]
		p.refilter()
		p.resetCursorToFirstVisible()
	}
default:
	// Printable rune input goes to filter.
	if msg.Type == tea.KeyRunes && len(msg.Runes) > 0 {
		p.filterText += string(msg.Runes)
		p.refilter()
		p.resetCursorToFirstVisible()
	}
}
```

Also update the preview-active key handling to use arrow keys only (remove `h`, `k`, `j` aliases):

```go
if p.previewActive {
	switch msg.String() {
	case "left", "esc":
		p.previewActive = false
		p.previewScroll = 0
		return p, nil
	case "up":
		if p.previewScroll > 0 {
			p.previewScroll--
		}
		return p, nil
	case "down":
		p.previewScroll++
		return p, nil
	}
	return p, nil
}
```

Add the `resetCursorToFirstVisible` helper:

```go
// resetCursorToFirstVisible moves the cursor to the first visible selectable item.
func (p *Picker) resetCursorToFirstVisible() {
	for i := range p.items {
		if !p.isSkippable(i) {
			p.cursor = i
			p.offset = 0
			p.clampScroll()
			return
		}
	}
	// If nothing visible, park cursor at 0.
	p.cursor = 0
	p.offset = 0
}
```

Update `doSelectAll` and `doSelectNone` to operate on filtered subset:

```go
func (p *Picker) doSelectAll() {
	for i := range p.items {
		if isSelectableItem(p.items[i]) && !p.isFilteredOut(i) {
			p.items[i].Selected = true
		}
	}
	p.syncSelectAll()
}

func (p *Picker) doSelectNone() {
	for i := range p.items {
		if isSelectableItem(p.items[i]) && !p.isFilteredOut(i) {
			p.items[i].Selected = false
		}
	}
	p.syncSelectAll()
}
```

**Step 4: Run tests to verify they pass**

Run: `go test ./cmd/claude-sync/tui/ -run "TestFilter|TestEsc|TestCtrl|TestArrow" -v`
Expected: PASS

**Step 5: Run full test suite**

Run: `go test ./cmd/claude-sync/tui/ -v`
Expected: All PASS. Some existing tests may need updates if they rely on `a`/`n`/`h`/`esc` key behavior.

Note: The existing `TestHKeyInContentDoesNotOpenHelp` test sends `h` while in FocusContent. With the new behavior, `h` in the picker types "h" into the filter instead of emitting FocusChangeMsg to sidebar. The root.go `esc` handler catches the sidebar focus change on first Esc. This test should still pass because the root.go `esc` handler runs before the picker — verify this.

Also: `TestEscFromContentGoesToSidebar` sends Esc from content zone. With filter changes, the picker's Esc first checks if filter is non-empty. Since the test doesn't type anything into the filter, filter is empty, so Esc still emits FocusChangeMsg. But wait — the root.go `esc` handler at line 297 intercepts `esc` from FocusContent before it reaches the picker. So this should still work. Verify.

**Step 6: Commit**

```bash
git add cmd/claude-sync/tui/picker.go cmd/claude-sync/tui/picker_test.go
git commit -m "feat: add filter keyboard handling to picker"
```

---

### Task 3: Update View() to render filter bar and filtered items

**Files:**
- Modify: `cmd/claude-sync/tui/picker.go`
- Test: `cmd/claude-sync/tui/picker_test.go`

**Step 1: Write the failing tests**

Add to `picker_test.go`:

```go
func TestViewShowsFilterBar(t *testing.T) {
	p := NewPicker(testItems())
	p.focused = true
	p.SetHeight(20)
	p.SetWidth(60)

	view := p.View()
	assert.Contains(t, view, "Filter:", "view should contain filter bar")
}

func TestViewFilterBarShowsCount(t *testing.T) {
	p := NewPicker(testItems())
	p.focused = true
	p.SetHeight(20)
	p.SetWidth(60)
	p.filterText = "alpha"
	p.refilter()

	view := p.View()
	assert.Contains(t, view, "1/3", "should show filtered/total count")
}

func TestViewShowsNoMatches(t *testing.T) {
	p := NewPicker(testItems())
	p.focused = true
	p.SetHeight(20)
	p.SetWidth(60)
	p.filterText = "zzzzz"
	p.refilter()

	view := p.View()
	assert.Contains(t, view, "No matches", "should show no matches message")
}

func TestViewHidesFilteredItems(t *testing.T) {
	p := NewPicker(testItems())
	p.focused = true
	p.SetHeight(20)
	p.SetWidth(60)
	p.filterText = "alpha"
	p.refilter()

	view := p.View()
	assert.Contains(t, view, "alpha", "matching item should be visible")
	assert.NotContains(t, view, "beta", "non-matching item should be hidden")
	assert.NotContains(t, view, "charlie", "non-matching item should be hidden")
}

func TestViewSearchActionAlwaysVisible(t *testing.T) {
	p := NewPicker(testItems())
	p.SetSearchAction(true)
	p.focused = true
	p.SetHeight(20)
	p.SetWidth(60)
	p.filterText = "zzzzz"
	p.refilter()

	view := p.View()
	assert.Contains(t, view, "Search projects", "search action should always be visible")
}
```

**Step 2: Run tests to verify they fail**

Run: `go test ./cmd/claude-sync/tui/ -run "TestView" -v`
Expected: FAIL (no filter bar rendered yet)

**Step 3: Implement View() changes**

The `View()` method needs significant changes. Replace the existing `View()` with a version that:
1. Renders the filter bar as the first line
2. Iterates `filterView` indices when filter is active (or all items when not)
3. Accounts for the filter bar taking 1 line of height
4. Shows "No matches" when filter yields 0 results
5. Always shows the search action row

Key change: the filter bar occupies 1 line, reducing available height for items by 1.

```go
func (p Picker) View() string {
	if p.previewActive && p.hasPreview {
		return p.viewWithPreview()
	}

	var b strings.Builder

	// Render filter bar (always visible, takes 1 line).
	b.WriteString(p.renderFilterBar())
	b.WriteString("\n")

	// Determine visible item indices.
	indices := p.viewIndices()
	totalRows := len(indices)
	if p.hasSearchAction {
		totalRows++
	}

	if totalRows == 0 {
		if p.filterText != "" {
			return ContentPaneStyle.Render(b.String() + "  No matches")
		}
		return ContentPaneStyle.Render(b.String() + "  (no items)")
	}

	// Available height for items (subtract 1 for filter bar).
	itemHeight := p.height - 1
	if itemHeight < 1 {
		itemHeight = 1
	}

	// Compute scroll offset based on cursor position within visible list.
	cursorPos := p.cursorVisiblePos(indices)
	offset := p.computeScrollOffset(cursorPos, totalRows, itemHeight)

	// Reserve lines for scroll indicators.
	visibleItems := itemHeight
	hasAbove := offset > 0
	hasBelow := offset+itemHeight < totalRows
	if hasAbove {
		visibleItems--
	}
	if hasBelow {
		visibleItems--
	}
	if visibleItems < 1 {
		visibleItems = 1
	}

	if hasAbove {
		b.WriteString(lipgloss.NewStyle().Foreground(colorOverlay0).Render("  ↑ more") + "\n")
	}

	end := offset + visibleItems
	if end > totalRows {
		end = totalRows
	}

	dimStyle := lipgloss.NewStyle().Foreground(colorOverlay0)

	for vi := offset; vi < end; vi++ {
		// Virtual search action row at the end of visible items.
		if vi == len(indices) && p.hasSearchAction {
			cursor := "  "
			realIdx := len(p.items) // virtual row
			if p.focused && p.cursor == realIdx {
				cursor = "> "
			}
			actionText := "[+ Search projects]"
			if p.focused && p.cursor == realIdx {
				actionText = lipgloss.NewStyle().Bold(true).Foreground(colorBlue).Render(actionText)
			} else if p.focused {
				actionText = lipgloss.NewStyle().Foreground(colorBlue).Render(actionText)
			} else {
				actionText = dimStyle.Render(actionText)
			}
			b.WriteString(cursor + actionText + "\n")
			continue
		}

		i := indices[vi] // real item index
		it := p.items[i]

		// Same rendering as before, but use real index i for cursor comparison.
		// (header, description, and item rendering unchanged from original View)
		if it.IsHeader {
			line := fmt.Sprintf("── %s ──", it.Display)
			if p.focused {
				b.WriteString(HeaderStyle.Render(line))
			} else {
				b.WriteString(dimStyle.Render(line))
			}
			b.WriteString("\n")
			continue
		}

		if it.Description != "" {
			b.WriteString("  " + dimStyle.Render(it.Description) + "\n")
			continue
		}

		cursor := "  "
		if p.focused && i == p.cursor {
			cursor = "> "
		}

		var checkbox string
		if p.focused {
			if it.Selected {
				checkbox = SelectedStyle.Render("[x]")
			} else {
				checkbox = UnselectedStyle.Render("[ ]")
			}
		} else {
			if it.Selected {
				checkbox = dimStyle.Render("[x]")
			} else {
				checkbox = dimStyle.Render("[ ]")
			}
		}

		var display string
		if p.focused && i == p.cursor {
			display = lipgloss.NewStyle().Bold(true).Foreground(colorText).Render(it.Display)
		} else if p.focused {
			display = it.Display
		} else {
			display = dimStyle.Render(it.Display)
		}

		tag := ""
		if it.Tag != "" {
			if p.focused && p.tagColor != "" {
				tag = "  " + lipgloss.NewStyle().Foreground(p.tagColor).Render(it.Tag)
			} else if p.focused {
				tag = "  " + BaseTagStyle.Render(it.Tag)
			} else {
				tag = "  " + dimStyle.Render(it.Tag)
			}
		}

		b.WriteString(cursor + checkbox + " " + display + tag + "\n")
	}

	if end < totalRows {
		b.WriteString(lipgloss.NewStyle().Foreground(colorOverlay0).Render("  ↓ more") + "\n")
	}

	return ContentPaneStyle.Render(strings.TrimRight(b.String(), "\n"))
}
```

Add helper methods:

```go
// viewIndices returns the list of item indices to render.
// When no filter is active, returns all item indices.
func (p Picker) viewIndices() []int {
	if p.filterView != nil {
		return p.filterView
	}
	indices := make([]int, len(p.items))
	for i := range p.items {
		indices[i] = i
	}
	return indices
}

// cursorVisiblePos returns the cursor's position within the visible indices.
func (p Picker) cursorVisiblePos(indices []int) int {
	for vi, i := range indices {
		if i == p.cursor {
			return vi
		}
	}
	// Cursor is on search action row.
	if p.hasSearchAction && p.cursor == len(p.items) {
		return len(indices)
	}
	return 0
}

// computeScrollOffset computes the scroll offset to keep cursorPos visible.
func (p Picker) computeScrollOffset(cursorPos, totalRows, viewHeight int) int {
	effectiveHeight := viewHeight
	if totalRows > viewHeight {
		effectiveHeight -= 2 // scroll indicators
	}
	if effectiveHeight < 1 {
		effectiveHeight = 1
	}

	offset := p.offset
	// Adjust offset based on cursor position in the visible list.
	if cursorPos < offset {
		offset = cursorPos
	}
	if cursorPos >= offset+effectiveHeight {
		offset = cursorPos - effectiveHeight + 1
	}
	maxOffset := totalRows - effectiveHeight
	if maxOffset < 0 {
		maxOffset = 0
	}
	if offset > maxOffset {
		offset = maxOffset
	}
	return offset
}

// renderFilterBar renders the filter input line.
func (p Picker) renderFilterBar() string {
	dimStyle := lipgloss.NewStyle().Foreground(colorOverlay0)
	label := dimStyle.Render("Filter: ")

	filterDisplay := p.filterText
	cursor := dimStyle.Render("_")
	if p.focused {
		cursor = lipgloss.NewStyle().Foreground(colorText).Render("_")
	}

	right := ""
	if p.filterText != "" {
		visible := p.visibleSelectableCount()
		total := p.TotalCount()
		right = dimStyle.Render(fmt.Sprintf("%d/%d", visible, total))
	}

	left := label + filterDisplay + cursor
	if right != "" {
		// Right-align the count.
		gap := p.width - lipgloss.Width(left) - lipgloss.Width(right) - 2
		if gap < 1 {
			gap = 1
		}
		return " " + left + strings.Repeat(" ", gap) + right
	}
	return " " + left
}
```

Also update `clampScroll` to work with visible items:

```go
func (p *Picker) clampScroll() {
	if p.height <= 0 {
		return
	}

	indices := p.viewIndices()
	totalRows := len(indices)
	if p.hasSearchAction {
		totalRows++
	}

	// Filter bar takes 1 line.
	itemHeight := p.height - 1
	if itemHeight < 1 {
		itemHeight = 1
	}

	effectiveHeight := itemHeight
	if totalRows > itemHeight {
		effectiveHeight -= 2
	}
	if effectiveHeight < 1 {
		effectiveHeight = 1
	}

	cursorPos := p.cursorVisiblePos(indices)
	if cursorPos < p.offset {
		p.offset = cursorPos
	}
	if cursorPos >= p.offset+effectiveHeight {
		p.offset = cursorPos - effectiveHeight + 1
	}
	maxOffset := totalRows - effectiveHeight
	if maxOffset < 0 {
		maxOffset = 0
	}
	if p.offset > maxOffset {
		p.offset = maxOffset
	}
}
```

Update `moveCursor` to work with visible indices:

```go
func (p *Picker) moveCursor(dir int) {
	indices := p.viewIndices()

	// Find current position in visible list.
	curPos := -1
	for vi, i := range indices {
		if i == p.cursor {
			curPos = vi
			break
		}
	}
	// Handle search action row.
	if p.hasSearchAction && p.cursor == len(p.items) {
		curPos = len(indices)
	}

	// Move within visible items, skipping headers/descriptions.
	nextPos := curPos + dir
	for nextPos >= 0 && nextPos <= len(indices) {
		// Search action row.
		if nextPos == len(indices) && p.hasSearchAction {
			p.cursor = len(p.items)
			p.clampScroll()
			return
		}
		if nextPos < len(indices) {
			realIdx := indices[nextPos]
			if !p.items[realIdx].IsHeader && p.items[realIdx].Description == "" {
				p.cursor = realIdx
				p.clampScroll()
				return
			}
		}
		nextPos += dir
	}
}
```

**Step 4: Run tests to verify they pass**

Run: `go test ./cmd/claude-sync/tui/ -run "TestView" -v`
Expected: PASS

**Step 5: Run full test suite**

Run: `go test ./cmd/claude-sync/tui/ -v`
Expected: All PASS. Note: `TestViewLineCount_*` tests may need height adjustments since the filter bar takes 1 line. If they fail, the filter bar's 1-line overhead is already accounted for in `clampScroll`'s `itemHeight = p.height - 1` calculation, but the `clampHeight` call in `currentContentView` might need to pass `contentHeight` that accounts for the filter bar. Investigate and fix if needed.

**Step 6: Commit**

```bash
git add cmd/claude-sync/tui/picker.go cmd/claude-sync/tui/picker_test.go
git commit -m "feat: render filter bar and filtered items in picker"
```

---

### Task 4: Update helper text in root.go

**Files:**
- Modify: `cmd/claude-sync/tui/root.go`
- Test: `cmd/claude-sync/tui/root_test.go`

**Step 1: Identify helper text to change**

In `root.go`, the `helperText` function returns line2 shortcuts. Current values:
- Default: `"Space: toggle · a: all · n: none"`
- ClaudeMD: `"Space: toggle · a: all · n: none · →: preview content"`
- CommandsSkills: `"Space: toggle · a: all · n: none · →: preview"`
- Keybindings: `"Space: toggle"`

New values:
- Default: `"Space: toggle · Ctrl+A: all · Ctrl+N: none · type to filter"`
- ClaudeMD: `"Space: toggle · a: all · n: none · →: preview content"` (unchanged — Preview component, not Picker)
- CommandsSkills: `"Space: toggle · Ctrl+A: all · Ctrl+N: none · →: preview · type to filter"`
- Keybindings: `"Space: toggle · type to filter"`

**Step 2: Make the changes**

In `root.go`, update the line2 switch in `helperText`:

```go
var line2 string
switch section {
case SectionClaudeMD:
	line2 = "Space: toggle · a: all · n: none · →: preview content"
case SectionCommandsSkills:
	line2 = "Space: toggle · Ctrl+A: all · Ctrl+N: none · →: preview · type to filter"
case SectionKeybindings:
	line2 = "Space: toggle · type to filter"
default:
	line2 = "Space: toggle · Ctrl+A: all · Ctrl+N: none · type to filter"
}
```

**Step 3: Update tests**

In `root_test.go`, the test `TestRenderHelper_Compact` asserts:
```go
assert.NotContains(t, result, "Space: toggle")
```
This checks that compact mode (2 lines) doesn't include the shortcuts line. This should still pass since line2 is only rendered when `lines >= 3`.

The test `TestRenderHelper_NoWrap` uses `SectionClaudeMD` at width 30 with 3 lines. The CLAUDE.md line is unchanged so this should still pass.

Check if `TestHelperTextPerSection` or `TestHelperTextProfileVsBase` need updates. They just assert non-empty, so they should pass.

**Step 4: Run tests**

Run: `go test ./cmd/claude-sync/tui/ -v`
Expected: All PASS

**Step 5: Commit**

```bash
git add cmd/claude-sync/tui/root.go
git commit -m "feat: update helper text for filter keybindings"
```

---

### Task 5: Update root.go Esc handling

**Files:**
- Modify: `cmd/claude-sync/tui/root.go`

**Context:** The root model's `Update()` intercepts `esc` from `FocusContent` at line 297 before the picker ever sees it. This means the picker's filter-clearing Esc logic won't fire. We need to change the root to delegate Esc to the picker first when filter is active.

**Step 1: Modify the Esc handler in root.go**

Replace the `esc` case in the global key handler:

```go
case "esc":
	if m.focusZone == FocusContent || m.focusZone == FocusPreview {
		// If the active picker has a filter, let the picker handle Esc first
		// (clears filter). Only go to sidebar if filter was already empty.
		if m.activeSection != SectionClaudeMD {
			p := m.currentPicker()
			if p.filterText != "" {
				// Delegate to picker — it will clear the filter.
				return m.updatePicker(msg, &cmds)
			}
		}
		m.focusZone = FocusSidebar
		return m, nil
	}
	// Esc on sidebar → quit confirmation.
	m.overlay = NewConfirmOverlay("Quit", "Discard selections and exit?")
	m.overlayCtx = overlayQuitConfirm
	return m, nil
```

Note: This requires access to `cmds` which is declared at the top of Update(). Move the `var cmds []tea.Cmd` above the switch so it's accessible in the esc handler. Actually, looking at the code, the esc handler is inside the `if msg, ok := msg.(tea.KeyMsg)` block which is before the focus-based routing. We need to pass a `cmds` slice. Since the esc handler currently returns directly, we can create a local one:

```go
case "esc":
	if m.focusZone == FocusContent || m.focusZone == FocusPreview {
		if m.activeSection != SectionClaudeMD {
			p := m.currentPicker()
			if p.filterText != "" {
				var escCmds []tea.Cmd
				return m.updatePicker(msg, &escCmds)
			}
		}
		m.focusZone = FocusSidebar
		return m, nil
	}
	m.overlay = NewConfirmOverlay("Quit", "Discard selections and exit?")
	m.overlayCtx = overlayQuitConfirm
	return m, nil
```

**Step 2: Run tests**

Run: `go test ./cmd/claude-sync/tui/ -v`
Expected: All PASS. `TestEscFromContentGoesToSidebar` should still pass because it doesn't set a filter.

**Step 3: Commit**

```bash
git add cmd/claude-sync/tui/root.go
git commit -m "fix: delegate Esc to picker for filter clearing before sidebar focus"
```

---

### Task 6: Handle viewWithPreview filter bar

**Files:**
- Modify: `cmd/claude-sync/tui/picker.go`

The `viewWithPreview()` method renders the split view when preview is active. It should also show the filter bar and only list visible items on the left side.

**Step 1: Update viewWithPreview**

Add filter bar at the top. The compact list on the left should only show visible items. Since preview disables typing, the filter bar is display-only.

In `viewWithPreview()`, add filter bar rendering before the list:

```go
func (p Picker) viewWithPreview() string {
	// ... existing width calculations ...

	// Render filter bar first.
	var topBar string
	if p.filterText != "" {
		dimStyle := lipgloss.NewStyle().Foreground(colorOverlay0)
		topBar = " " + dimStyle.Render("Filter: ") + p.filterText + "\n"
	}

	// ... existing preview content code ...

	// Compact list: only show visible items.
	indices := p.viewIndices()
	var listB strings.Builder
	dimStyle := lipgloss.NewStyle().Foreground(colorOverlay0)
	for _, idx := range indices {
		it := p.items[idx]
		if it.IsHeader || it.Description != "" {
			continue
		}
		cursor := "  "
		if p.focused && idx == p.cursor {
			cursor = "> "
		}
		display := it.Display
		if len(display) > listWidth-4 {
			display = display[:listWidth-7] + "..."
		}
		if p.focused && idx == p.cursor {
			display = lipgloss.NewStyle().Bold(true).Foreground(colorText).Render(display)
		} else if !p.focused {
			display = dimStyle.Render(display)
		}
		listB.WriteString(cursor + display + "\n")
	}
	listView := lipgloss.NewStyle().Width(listWidth).Render(
		strings.TrimRight(listB.String(), "\n"),
	)

	joined := lipgloss.JoinHorizontal(lipgloss.Top, listView, previewView)
	if topBar != "" {
		return topBar + joined
	}
	return joined
}
```

**Step 2: Run tests**

Run: `go test ./cmd/claude-sync/tui/ -v`
Expected: All PASS

**Step 3: Commit**

```bash
git add cmd/claude-sync/tui/picker.go
git commit -m "feat: show filter bar in preview split view"
```

---

### Task 7: Integration testing and height verification

**Files:**
- Test: `cmd/claude-sync/tui/root_test.go`
- Modify: `cmd/claude-sync/tui/root.go` (if height adjustments needed)

**Step 1: Run all view line count tests**

Run: `go test ./cmd/claude-sync/tui/ -run "TestViewLineCount" -v`

The filter bar adds 1 line to the picker output. The `currentContentView` in `root.go` calls `clampHeight(view, height)` which truncates. Since the picker's `View()` now accounts for the filter bar in its own height calculation (`itemHeight = p.height - 1`), the total output of `View()` should still fit within `p.height` lines. Verify this.

If tests fail, adjust `distributeSize()` in root.go to pass `contentHeight - 1` to picker heights (accounting for filter bar), or adjust the picker to not count the filter bar against its height. The cleanest fix: the picker handles it internally — its `View()` produces exactly `p.height` lines (filter bar + items).

**Step 2: Run full test suite**

Run: `go test ./... `
Expected: All PASS

**Step 3: Commit if any fixes were needed**

```bash
git add cmd/claude-sync/tui/
git commit -m "fix: adjust heights for filter bar in picker"
```

---

### Task 8: Final build and manual verification

**Step 1: Build**

Run: `go build ./...`
Expected: Clean build

**Step 2: Install and test interactively**

```bash
go build -o ~/.local/bin/claude-sync -ldflags "-X main.version=0.3.1" ./cmd/claude-sync/
```

**Step 3: Manual verification checklist**

- [ ] `claude-sync config create` shows filter bar in every section
- [ ] Typing narrows the list in real-time
- [ ] Backspace removes characters
- [ ] Esc clears filter (with filter active)
- [ ] Esc goes to sidebar (with filter empty)
- [ ] Arrow keys navigate the filtered list
- [ ] Ctrl+A selects all visible items
- [ ] Ctrl+N deselects all visible items
- [ ] Space toggles the current item
- [ ] `→` opens preview (when filter is empty, on Cmds & Skills)
- [ ] Filter bar shows N/M count when active
- [ ] "No matches" appears when filter yields nothing
- [ ] Headers auto-hide when they have no matching children
- [ ] `[+ Search projects]` is always visible
- [ ] Helper text shows updated shortcuts

**Step 4: Commit**

```bash
git add -A
git commit -m "chore: final filter bar polish"
```
