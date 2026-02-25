package tui

import (
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"

	tea "github.com/charmbracelet/bubbletea"
)

// ReviewResult holds the user's accept/skip decisions for new subscription items.
type ReviewResult struct {
	Accepted map[string][]string // category -> accepted item names
	Skipped  map[string][]string // category -> skipped item names
}

// ReviewModel is a standalone TUI for reviewing new items from subscriptions.
type ReviewModel struct {
	subName  string
	newItems map[string][]string // category -> new item names
	picker   Picker
	width    int
	height   int
	ready    bool
	confirmed bool
	skipped   bool
}

// NewReviewModel creates a new subscription review TUI.
// newItems is a map of category -> new item names to review.
func NewReviewModel(subName string, newItems map[string][]string) ReviewModel {
	items := buildReviewItems(newItems)
	picker := NewPicker(items)
	picker.focused = true

	return ReviewModel{
		subName:  subName,
		newItems: newItems,
		picker:   picker,
	}
}

func (m ReviewModel) Init() tea.Cmd { return nil }

func (m ReviewModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.ready = true
		m.picker.SetHeight(m.height - 6)
		m.picker.SetWidth(m.width)
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			m.skipped = true
			return m, tea.Quit
		case "enter":
			m.confirmed = true
			return m, tea.Quit
		case "s": // skip all
			m.skipped = true
			return m, tea.Quit
		}
	}

	var cmd tea.Cmd
	m.picker, cmd = m.picker.Update(msg)
	return m, cmd
}

func (m ReviewModel) View() string {
	if !m.ready {
		return "Loading..."
	}

	var b strings.Builder

	// Header.
	header := lipgloss.NewStyle().
		Bold(true).
		Foreground(colorYellow).
		Render(fmt.Sprintf("New items from %s:", m.subName))
	b.WriteString(header + "\n\n")

	// Picker.
	b.WriteString(m.picker.View())

	// Footer.
	footer := lipgloss.NewStyle().
		Foreground(colorSubtext0).
		Render("\n  Space: toggle  •  Enter: apply selected  •  s: skip all  •  q: cancel")
	b.WriteString(footer)

	return b.String()
}

// Result returns the review result.
func (m ReviewModel) Result() *ReviewResult {
	if m.skipped {
		// Everything skipped.
		return &ReviewResult{
			Accepted: make(map[string][]string),
			Skipped:  m.newItems,
		}
	}
	if !m.confirmed {
		return nil
	}

	selected := make(map[string]bool)
	for _, key := range m.picker.SelectedKeys() {
		selected[key] = true
	}

	result := &ReviewResult{
		Accepted: make(map[string][]string),
		Skipped:  make(map[string][]string),
	}

	for cat, items := range m.newItems {
		for _, name := range items {
			key := cat + ":" + name
			if selected[key] {
				result.Accepted[cat] = append(result.Accepted[cat], name)
			} else {
				result.Skipped[cat] = append(result.Skipped[cat], name)
			}
		}
	}

	return result
}

func buildReviewItems(newItems map[string][]string) []PickerItem {
	var items []PickerItem

	// Sort categories for deterministic display.
	cats := make([]string, 0, len(newItems))
	for cat := range newItems {
		cats = append(cats, cat)
	}
	sort.Strings(cats)

	for _, cat := range cats {
		names := newItems[cat]
		if len(names) == 0 {
			continue
		}

		items = append(items, PickerItem{
			Display:  fmt.Sprintf("%s (%d new)", categoryDisplayName(cat), len(names)),
			IsHeader: true,
		})

		for _, name := range names {
			items = append(items, PickerItem{
				Key:      cat + ":" + name,
				Display:  name,
				Selected: true, // default: accept new items
				Tag:      "(new)",
			})
		}
	}

	return items
}
