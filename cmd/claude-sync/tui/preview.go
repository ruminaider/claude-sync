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

// Preview shows a two-panel layout for CLAUDE.md: section list on the left
// and content preview on the right. The last entry in the left panel is a
// [+ Search projects] action.
type Preview struct {
	sections    []PreviewSection
	cursor      int
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
	if len(sections) > 0 {
		vp.SetContent(sections[0].Content)
	}

	p := Preview{
		sections:  sections,
		cursor:    0,
		selected:  sel,
		viewport:  vp,
		listWidth: 30,
		focusLeft: true,
	}
	return p
}

// ClaudeMDPreviewSections converts claudemd.Section values from a scan into
// PreviewSection values suitable for the preview model.
func ClaudeMDPreviewSections(sections []claudemd.Section, source string) []PreviewSection {
	result := make([]PreviewSection, 0, len(sections))
	for _, sec := range sections {
		header := sec.Header
		if header == "" {
			header = "(preamble)"
		}
		result = append(result, PreviewSection{
			Header:      header,
			Content:     sec.Content,
			FragmentKey: claudemd.HeaderToFragmentName(sec.Header),
			Source:      source,
		})
	}
	return result
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
// them by default.
func (p *Preview) AddSections(sections []PreviewSection) {
	startIdx := len(p.sections)
	p.sections = append(p.sections, sections...)
	for i := startIdx; i < len(p.sections); i++ {
		p.selected[i] = true
	}
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
	// The "virtual" last entry is [+ Search projects].
	totalEntries := len(p.sections) + 1 // +1 for the search action

	switch msg.String() {
	case "up", "k":
		if p.cursor > 0 {
			p.cursor--
			p.syncViewport()
		}
	case "down", "j":
		if p.cursor < totalEntries-1 {
			p.cursor++
			p.syncViewport()
		}
	case " ":
		// Toggle selection — only on real sections, not the search action.
		if p.cursor < len(p.sections) {
			p.selected[p.cursor] = !p.selected[p.cursor]
		}
	case "enter":
		// If on the search action, emit SearchRequestMsg (unless already searching).
		if p.cursor == len(p.sections) && !p.searching {
			return p, func() tea.Msg { return SearchRequestMsg{} }
		}
		// Otherwise toggle selection.
		if p.cursor < len(p.sections) {
			p.selected[p.cursor] = !p.selected[p.cursor]
		}
	case "right", "l":
		p.focusLeft = false
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
// section. If the cursor is on the search action, the viewport shows a hint.
func (p *Preview) syncViewport() {
	if p.cursor < len(p.sections) {
		p.viewport.SetContent(p.wrapContent(p.sections[p.cursor].Content))
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

	dimStyle := lipgloss.NewStyle().Foreground(colorOverlay0)

	for i, sec := range p.sections {
		cursor := "  "
		if p.focused && i == p.cursor && p.focusLeft {
			cursor = "> "
		}

		var checkbox string
		if p.focused {
			if p.selected[i] {
				checkbox = SelectedStyle.Render("[x]")
			} else {
				checkbox = UnselectedStyle.Render("[ ]")
			}
		} else {
			if p.selected[i] {
				checkbox = dimStyle.Render("[x]")
			} else {
				checkbox = dimStyle.Render("[ ]")
			}
		}

		header := sec.Header
		// Truncate long headers to fit the list width.
		maxHeaderLen := p.listWidth - 10 // account for cursor + checkbox + padding
		if maxHeaderLen < 5 {
			maxHeaderLen = 5
		}
		if len(header) > maxHeaderLen {
			header = header[:maxHeaderLen-1] + "…"
		}

		var display string
		if p.focused && i == p.cursor && p.focusLeft {
			display = lipgloss.NewStyle().Bold(true).Foreground(colorText).Render(header)
		} else if p.focused {
			display = header
		} else {
			display = dimStyle.Render(header)
		}

		tag := ""
		if sec.IsBase {
			if p.focused {
				tag = "  " + BaseTagStyle.Render("[base]")
			} else {
				tag = "  " + dimStyle.Render("[base]")
			}
		}

		b.WriteString(cursor + checkbox + " " + display + tag + "\n")
	}

	// [+ Search projects] action row.
	searchCursor := "  "
	if p.focused && p.cursor == len(p.sections) && p.focusLeft {
		searchCursor = "> "
	}
	var searchLabel string
	if p.searching {
		searchLabel = "Searching..."
		if p.focused && p.cursor == len(p.sections) && p.focusLeft {
			searchLabel = lipgloss.NewStyle().Bold(true).Foreground(colorOverlay0).Render(searchLabel)
		} else {
			searchLabel = dimStyle.Render(searchLabel)
		}
	} else {
		searchLabel = lipgloss.NewStyle().Foreground(colorBlue).Render("[+ Search projects]")
		if !p.focused {
			searchLabel = dimStyle.Render("[+ Search projects]")
		}
	}
	b.WriteString(searchCursor + searchLabel + "\n")

	return listStyle.Height(p.totalHeight).Render(b.String())
}

func (p Preview) viewPreview() string {
	// Header for the preview panel.
	var header string
	if p.cursor < len(p.sections) {
		sec := p.sections[p.cursor]
		title := sec.Header
		if sec.Source != "" {
			title = fmt.Sprintf("%s  (%s)", title, sec.Source)
		}
		header = HeaderStyle.Render(title) + "\n"
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
