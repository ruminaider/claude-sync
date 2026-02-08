package main

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// picker is a multi-select TUI where Enter toggles items and confirms at the bottom.
type picker struct {
	title    string
	items    []string
	selected map[int]bool
	cursor   int
	done     bool
}

func newPicker(title string, items []string) picker {
	return picker{
		title:    title,
		items:    items,
		selected: make(map[int]bool),
		cursor:   0,
	}
}

func (p picker) Init() tea.Cmd { return nil }

func (p picker) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q", "esc":
			p.selected = nil
			p.done = true
			return p, tea.Quit
		case "up", "k":
			if p.cursor > 0 {
				p.cursor--
			}
		case "down", "j":
			if p.cursor < len(p.items) {
				p.cursor++
			}
		case "enter":
			if p.cursor == len(p.items) {
				// Confirm button
				p.done = true
				return p, tea.Quit
			}
			// Toggle item
			p.selected[p.cursor] = !p.selected[p.cursor]
		case "a":
			// Select all
			for i := range p.items {
				p.selected[i] = true
			}
		case "n":
			// Select none
			for i := range p.items {
				p.selected[i] = false
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

// Selected returns the selected items, or nil if cancelled.
func (p picker) Selected() []string {
	if p.selected == nil {
		return nil
	}
	var result []string
	for i, item := range p.items {
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
	return model.(picker).Selected(), nil
}
