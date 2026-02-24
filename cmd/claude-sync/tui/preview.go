package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/ruminaider/claude-sync/internal/claudemd"
)

// SearchRequestMsg is emitted when the user activates the [+ Search projects]
// action in the CLAUDE.md preview panel.
type SearchRequestMsg struct{}

// PreviewSection represents one section of a CLAUDE.md file in the preview.
type PreviewSection struct {
	Header      string // display name: "(preamble)" for empty header, otherwise the header text
	Content     string // full section content
	FragmentKey string // fragment name via HeaderToFragmentName
	Source      string // source file path (for grouping)
	IsBase      bool   // inherited from base (profile view)
}

// previewRow represents a single navigable entry in the virtual row list.
// It is either a group header or a reference to a section.
type previewRow struct {
	isHeader   bool
	source     string // group label (headers only)
	count      int    // number of sections in group (headers only)
	sectionIdx int    // index into p.sections (non-headers only)
}

// Preview shows a two-panel layout for CLAUDE.md: section list on the left
// and content preview on the right. The last entry in the left panel is a
// [+ Search projects] action.
type Preview struct {
	sections    []PreviewSection
	rows        []previewRow     // computed virtual row list
	rowCursor   int              // cursor position in rows
	collapsed   map[string]bool  // collapsed groups keyed by Source
	selected    map[int]bool
	viewport    viewport.Model
	listWidth   int
	totalWidth  int
	totalHeight int
	focusLeft   bool // true = list focused, false = preview focused
	focused     bool // true when this preview has keyboard focus
	searching   bool // true while a background search is running
}

// NewPreview creates a Preview model from a slice of PreviewSection values.
// All sections are pre-selected.
func NewPreview(sections []PreviewSection) Preview {
	sel := make(map[int]bool, len(sections))
	for i := range sections {
		sel[i] = true
	}

	vp := viewport.New(40, 10)

	p := Preview{
		sections:  sections,
		collapsed: make(map[string]bool),
		selected:  sel,
		viewport:  vp,
		listWidth: 30,
		focusLeft: true,
	}
	p.rebuildRows()
	p.syncViewport()
	return p
}

// ClaudeMDPreviewSections converts claudemd.Section values from a scan into
// PreviewSection values suitable for the preview model.
func ClaudeMDPreviewSections(sections []claudemd.Section, source string) []PreviewSection {
	result := make([]PreviewSection, 0, len(sections))
	isGlobal := strings.HasSuffix(source, "/.claude/CLAUDE.md")
	for _, sec := range sections {
		header := sec.Header
		if header == "" {
			header = "(preamble)"
		}
		fragKey := claudemd.HeaderToFragmentName(sec.Header)
		if !isGlobal {
			fragKey = source + "::" + fragKey
		}
		result = append(result, PreviewSection{
			Header:      header,
			Content:     sec.Content,
			FragmentKey: fragKey,
			Source:      source,
		})
	}
	return result
}

// rebuildRows computes the virtual row list from p.sections, grouping by Source
// and respecting the collapsed state. Must be called whenever sections change
// or a group is toggled.
func (p *Preview) rebuildRows() {
	p.rows = p.rows[:0]
	if len(p.sections) == 0 {
		return
	}

	// Group sections by source in order of first appearance.
	type group struct {
		source  string
		indices []int
	}
	var groups []group
	seen := make(map[string]int) // source -> index in groups

	for i, sec := range p.sections {
		if idx, ok := seen[sec.Source]; ok {
			groups[idx].indices = append(groups[idx].indices, i)
		} else {
			seen[sec.Source] = len(groups)
			groups = append(groups, group{
				source:  sec.Source,
				indices: []int{i},
			})
		}
	}

	for _, g := range groups {
		p.rows = append(p.rows, previewRow{
			isHeader: true,
			source:   g.source,
			count:    len(g.indices),
		})
		if !p.collapsed[g.source] {
			for _, idx := range g.indices {
				p.rows = append(p.rows, previewRow{
					sectionIdx: idx,
				})
			}
		}
	}
}

// --- Methods ---

// SelectedFragmentKeys returns the fragment keys of selected sections.
func (p Preview) SelectedFragmentKeys() []string {
	var keys []string
	for i, sec := range p.sections {
		if p.selected[i] {
			keys = append(keys, sec.FragmentKey)
		}
	}
	return keys
}

// SelectedCount returns the number of selected sections.
func (p Preview) SelectedCount() int {
	n := 0
	for i := range p.sections {
		if p.selected[i] {
			n++
		}
	}
	return n
}

// TotalCount returns the total number of sections.
func (p Preview) TotalCount() int {
	return len(p.sections)
}

// GlobalSelectedFragmentKeys returns the fragment keys of selected sections
// that belong to the global CLAUDE.md (unqualified keys without "::").
func (p Preview) GlobalSelectedFragmentKeys() []string {
	var keys []string
	for i, sec := range p.sections {
		if p.selected[i] && !strings.Contains(sec.FragmentKey, "::") {
			keys = append(keys, sec.FragmentKey)
		}
	}
	return keys
}

// ProjectSelectedFragmentKeys returns the fragment keys of selected sections
// that belong to project-level CLAUDE.md files (qualified keys with "::").
func (p Preview) ProjectSelectedFragmentKeys() []string {
	var keys []string
	for i, sec := range p.sections {
		if p.selected[i] && strings.Contains(sec.FragmentKey, "::") {
			keys = append(keys, sec.FragmentKey)
		}
	}
	return keys
}

// GlobalTotalCount returns the number of sections from the global CLAUDE.md
// (those with unqualified fragment keys).
func (p Preview) GlobalTotalCount() int {
	count := 0
	for _, sec := range p.sections {
		if !strings.Contains(sec.FragmentKey, "::") {
			count++
		}
	}
	return count
}

// SetSize sets the total dimensions available for the preview and recalculates
// the split between list and viewport panels.
func (p *Preview) SetSize(width, height int) {
	p.totalWidth = width
	p.totalHeight = height

	// Left panel gets roughly 1/3, right panel gets 2/3.
	p.listWidth = width / 3
	if p.listWidth < 20 {
		p.listWidth = 20
	}

	vpWidth := width - p.listWidth - 3 // 3 for border/divider padding
	if vpWidth < 10 {
		vpWidth = 10
	}
	vpHeight := height
	if vpHeight < 1 {
		vpHeight = 1
	}

	p.viewport.Width = vpWidth
	p.viewport.Height = vpHeight

	// Re-wrap content for the new width.
	p.syncViewport()
}

// SetFocused sets whether this preview currently has keyboard focus.
func (p *Preview) SetFocused(f bool) {
	p.focused = f
}

// AddSections appends new sections (e.g. from search results) and selects
// them by default. Sections whose Source already exists are skipped to
// prevent duplicates on repeated searches.
func (p *Preview) AddSections(sections []PreviewSection) {
	existing := make(map[string]bool)
	for _, s := range p.sections {
		if s.Source != "" {
			existing[s.Source] = true
		}
	}

	for _, s := range sections {
		if s.Source != "" && existing[s.Source] {
			continue
		}
		idx := len(p.sections)
		p.sections = append(p.sections, s)
		p.selected[idx] = true
	}

	p.rebuildRows()
}

// Update handles key messages for the preview model.
func (p Preview) Update(msg tea.Msg) (Preview, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if p.focusLeft {
			return p.updateList(msg)
		}
		return p.updateViewport(msg)
	}
	return p, nil
}

func (p Preview) updateList(msg tea.KeyMsg) (Preview, tea.Cmd) {
	// The "virtual" last entry after all rows is [+ Search projects].
	totalEntries := len(p.rows) + 1 // +1 for the search action

	switch msg.String() {
	case "up", "k":
		if p.rowCursor > 0 {
			p.rowCursor--
			p.syncViewport()
		}
	case "down", "j":
		if p.rowCursor < totalEntries-1 {
			p.rowCursor++
			p.syncViewport()
		}
	case " ":
		if p.rowCursor < len(p.rows) {
			row := p.rows[p.rowCursor]
			if row.isHeader {
				p.toggleCollapse(row.source)
			} else {
				p.selected[row.sectionIdx] = !p.selected[row.sectionIdx]
			}
		}
	case "enter":
		// If on the search action, emit SearchRequestMsg.
		if p.rowCursor == len(p.rows) && !p.searching {
			return p, func() tea.Msg { return SearchRequestMsg{} }
		}
		if p.rowCursor < len(p.rows) {
			row := p.rows[p.rowCursor]
			if row.isHeader {
				p.toggleCollapse(row.source)
			} else {
				p.selected[row.sectionIdx] = !p.selected[row.sectionIdx]
			}
		}
	case "a":
		// Select all sections regardless of collapse state.
		for i := range p.sections {
			p.selected[i] = true
		}
	case "n":
		// Deselect all sections regardless of collapse state.
		for i := range p.sections {
			p.selected[i] = false
		}
	case "right", "l":
		// Only switch to viewport focus on section rows, not headers.
		if p.rowCursor < len(p.rows) && !p.rows[p.rowCursor].isHeader {
			p.focusLeft = false
		}
	case "left", "h":
		return p, func() tea.Msg {
			return FocusChangeMsg{Zone: FocusSidebar}
		}
	case "esc":
		return p, func() tea.Msg {
			return FocusChangeMsg{Zone: FocusSidebar}
		}
	}
	return p, nil
}

// toggleCollapse flips the collapsed state for a source group and rebuilds rows.
func (p *Preview) toggleCollapse(source string) {
	p.collapsed[source] = !p.collapsed[source]
	p.rebuildRows()
	// Clamp cursor if it fell past the end.
	maxCursor := len(p.rows) // search action is at len(p.rows)
	if p.rowCursor > maxCursor {
		p.rowCursor = maxCursor
	}
}

func (p Preview) updateViewport(msg tea.KeyMsg) (Preview, tea.Cmd) {
	switch msg.String() {
	case "left", "h":
		p.focusLeft = true
		return p, nil
	case "esc":
		return p, func() tea.Msg {
			return FocusChangeMsg{Zone: FocusSidebar}
		}
	}

	// Delegate scrolling to the viewport.
	var cmd tea.Cmd
	p.viewport, cmd = p.viewport.Update(msg)
	return p, cmd
}

// syncViewport updates the viewport content to match the currently highlighted
// row. Headers show a summary; section rows show content; the search action
// shows a hint.
func (p *Preview) syncViewport() {
	if p.rowCursor < len(p.rows) {
		row := p.rows[p.rowCursor]
		if row.isHeader {
			info := fmt.Sprintf("%s\n\n%d section(s)", row.source, row.count)
			p.viewport.SetContent(p.wrapContent(info))
		} else {
			p.viewport.SetContent(p.wrapContent(p.sections[row.sectionIdx].Content))
		}
		p.viewport.GotoTop()
	} else {
		p.viewport.SetContent(p.wrapContent("Search for CLAUDE.md files in your projects..."))
		p.viewport.GotoTop()
	}
}

// wrapContent soft-wraps text to fit within the viewport width so that long
// lines are not clipped by the viewport boundary.
func (p *Preview) wrapContent(s string) string {
	w := p.viewport.Width
	if w <= 0 {
		return s
	}
	return lipgloss.NewStyle().Width(w).Render(s)
}

// View renders the two-panel split layout.
func (p Preview) View() string {
	left := p.viewList()
	right := p.viewPreview()

	// Divider between panels.
	divider := lipgloss.NewStyle().
		Foreground(colorSurface1).
		Render(strings.TrimRight(strings.Repeat("│\n", p.totalHeight), "\n"))

	return lipgloss.JoinHorizontal(lipgloss.Top, left, divider, right)
}

func (p Preview) viewList() string {
	var b strings.Builder

	listStyle := lipgloss.NewStyle().
		Width(p.listWidth).
		PaddingLeft(1)

	for ri, row := range p.rows {
		isCurrent := p.focused && ri == p.rowCursor && p.focusLeft

		if row.isHeader {
			// Render group header: ── ▾ source (N) ──
			indicator := "▾"
			if p.collapsed[row.source] {
				indicator = "▸"
			}
			line := fmt.Sprintf("── %s %s (%d) ──", indicator, row.source, row.count)

			cursor := "  "
			if isCurrent {
				cursor = "> "
			}

			b.WriteString(cursor + RenderHeader(line, p.focused, isCurrent))
			b.WriteString("\n")
			continue
		}

		// Section row.
		i := row.sectionIdx
		sec := p.sections[i]

		cursor := "  "
		if isCurrent {
			cursor = "> "
		}

		checkbox := RenderCheckbox(p.focused, p.selected[i])

		header := sec.Header
		// Truncate long headers to fit the list width.
		maxHeaderLen := p.listWidth - 10 // account for cursor + checkbox + padding
		if maxHeaderLen < 5 {
			maxHeaderLen = 5
		}
		if len(header) > maxHeaderLen {
			header = header[:maxHeaderLen-1] + "…"
		}

		// Removed inherited section: muted red strikethrough.
		if sec.IsBase && !p.selected[i] {
			b.WriteString(cursor + RenderRemovedBaseLine(header, "●", p.focused) + "\n")
			continue
		}

		display := RenderItemText(header, p.focused, isCurrent)

		tag := ""
		if sec.IsBase {
			tag = RenderTag("●", p.focused, "")
		}

		b.WriteString(cursor + checkbox + " " + display + tag + "\n")
	}

	// [+ Search projects] action row.
	searchIsCurrent := p.focused && p.rowCursor == len(p.rows) && p.focusLeft
	searchCursor := "  "
	if searchIsCurrent {
		searchCursor = "> "
	}
	b.WriteString(searchCursor + RenderSearchAction(p.focused, searchIsCurrent, p.searching) + "\n")

	return listStyle.Height(p.totalHeight).Render(b.String())
}

func (p Preview) viewPreview() string {
	// Header for the preview panel.
	var header string
	if p.rowCursor < len(p.rows) {
		row := p.rows[p.rowCursor]
		if row.isHeader {
			header = HeaderStyle.Render(row.source) + "\n"
		} else {
			sec := p.sections[row.sectionIdx]
			title := sec.Header
			if sec.Source != "" {
				title = fmt.Sprintf("%s  (%s)", title, sec.Source)
			}
			header = HeaderStyle.Render(title) + "\n"
		}
	} else {
		header = HeaderStyle.Render("Search") + "\n"
	}

	previewStyle := lipgloss.NewStyle().
		PaddingLeft(1).
		Width(p.viewport.Width + 1)

	vpHeight := p.totalHeight - 1 // minus 1 for the header line
	if vpHeight < 1 {
		vpHeight = 1
	}
	p.viewport.Height = vpHeight

	focusBorder := lipgloss.NewStyle()
	if !p.focusLeft {
		focusBorder = focusBorder.BorderLeft(true).
			BorderStyle(lipgloss.NormalBorder()).
			BorderForeground(colorBlue)
	}

	content := header + p.viewport.View()
	return focusBorder.Render(previewStyle.Height(p.totalHeight).Render(content))
}
