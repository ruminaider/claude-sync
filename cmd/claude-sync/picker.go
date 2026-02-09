package main

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
)

// pickerSection groups items under a header for display in the picker.
type pickerSection struct {
	Header string
	Items  []string
}

// picker is a multi-select TUI where Enter toggles items and confirms at the bottom.
type picker struct {
	title    string
	items    []string
	headers  map[int]bool // indices that are section headers (non-selectable)
	selected map[int]bool
	cursor   int
	done     bool
}

func newPicker(title string, items []string) picker {
	return picker{
		title:    title,
		items:    items,
		headers:  make(map[int]bool),
		selected: make(map[int]bool),
		cursor:   0,
	}
}

// newPickerWithSections creates a picker with section headers interleaved as non-selectable items.
// All non-header items are pre-selected by default (opt-out pattern).
func newPickerWithSections(title string, sections []pickerSection) picker {
	var items []string
	headers := make(map[int]bool)
	selected := make(map[int]bool)

	for _, sec := range sections {
		if len(sec.Items) == 0 {
			continue
		}
		idx := len(items)
		items = append(items, sec.Header)
		headers[idx] = true
		for _, item := range sec.Items {
			itemIdx := len(items)
			items = append(items, item)
			selected[itemIdx] = true // pre-select all non-header items
		}
	}

	// Start cursor on first non-header item.
	cursor := 0
	for cursor < len(items) && headers[cursor] {
		cursor++
	}

	return picker{
		title:    title,
		items:    items,
		headers:  headers,
		selected: selected,
		cursor:   cursor,
	}
}

func (p picker) Init() tea.Cmd { return nil }

// nextSelectable returns the next non-header index after current in the given direction.
// dir should be -1 (up) or +1 (down). Returns current if no valid move exists.
func (p picker) nextSelectable(current, dir int) int {
	next := current + dir
	for next >= 0 && next < len(p.items) {
		if !p.headers[next] {
			return next
		}
		next += dir
	}
	// If moving down past all items, allow landing on confirm button.
	if dir > 0 && next == len(p.items) {
		return next
	}
	return current
}

func (p picker) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q", "esc":
			p.selected = nil
			p.done = true
			return p, tea.Quit
		case "up", "k":
			if p.cursor == len(p.items) {
				// On confirm button — move up to last selectable item.
				p.cursor = p.nextSelectable(p.cursor, -1)
			} else if p.cursor > 0 {
				p.cursor = p.nextSelectable(p.cursor, -1)
			}
		case "down", "j":
			if p.cursor < len(p.items) {
				p.cursor = p.nextSelectable(p.cursor, +1)
			}
		case "enter":
			if p.cursor == len(p.items) {
				// Confirm button
				p.done = true
				return p, tea.Quit
			}
			if !p.headers[p.cursor] {
				p.selected[p.cursor] = !p.selected[p.cursor]
			}
		case "a":
			// Select all (skip headers)
			for i := range p.items {
				if !p.headers[i] {
					p.selected[i] = true
				}
			}
		case "n":
			// Select none (skip headers)
			for i := range p.items {
				if !p.headers[i] {
					p.selected[i] = false
				}
			}
		}
	}
	return p, nil
}

func (p picker) View() string {
	var b strings.Builder

	b.WriteString(fmt.Sprintf("  %s\n", p.title))
	b.WriteString("  a: select all · n: select none\n\n")

	for i, item := range p.items {
		if p.headers[i] {
			// Render section header without checkbox.
			b.WriteString(fmt.Sprintf("  ── %s ──\n", item))
			continue
		}
		cursor := "  "
		if p.cursor == i {
			cursor = "> "
		}
		check := "[ ]"
		if p.selected[i] {
			check = "[x]"
		}
		b.WriteString(fmt.Sprintf("  %s%s %s\n", cursor, check, item))
	}

	// Confirm button
	b.WriteString("\n")
	if p.cursor == len(p.items) {
		b.WriteString("  > [ Confirm ]\n")
	} else {
		b.WriteString("    [ Confirm ]\n")
	}

	return b.String()
}

// Selected returns the selected items (excluding headers), or nil if cancelled.
func (p picker) Selected() []string {
	if p.selected == nil {
		return nil
	}
	result := []string{}
	for i, item := range p.items {
		if p.headers[i] {
			continue
		}
		if p.selected[i] {
			result = append(result, item)
		}
	}
	return result
}

// runPicker runs the multi-select picker and returns selected items.
func runPicker(title string, items []string) ([]string, error) {
	p := newPicker(title, items)
	model, err := tea.NewProgram(p).Run()
	if err != nil {
		return nil, err
	}
	selected := model.(picker).Selected()
	if selected == nil {
		return nil, huh.ErrUserAborted
	}
	return selected, nil
}

// runPickerWithSections runs a multi-select picker with section headers.
func runPickerWithSections(title string, sections []pickerSection) ([]string, error) {
	p := newPickerWithSections(title, sections)
	model, err := tea.NewProgram(p).Run()
	if err != nil {
		return nil, err
	}
	selected := model.(picker).Selected()
	if selected == nil {
		return nil, huh.ErrUserAborted
	}
	return selected, nil
}
