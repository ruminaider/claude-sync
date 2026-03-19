package tui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/ruminaider/claude-sync/internal/commands"
)

type appView int

const (
	viewDashboard appView = iota
	viewActions
	viewSubView
)

// AppModel is the top-level Bubble Tea model that routes between
// dashboard, actions, and sub-view screens via a simple state machine.
type AppModel struct {
	state      commands.MenuState
	activeView appView
	width      int
	height     int
	version    string
	quitting   bool

	// Paths needed for executing actions later
	claudeDir string
	syncDir   string

	// cursor positions preserved per view
	dashboardScroll int
	actionCursor    int

	// sub-view state (populated when activeView == viewSubView)
	subView tea.Model
}

// NewAppModel creates an AppModel from detected state, starting on the dashboard.
func NewAppModel(state commands.MenuState) AppModel {
	return AppModel{
		state:      state,
		activeView: viewDashboard,
	}
}

// SetVersion sets the version string displayed in the header.
func (m *AppModel) SetVersion(v string) {
	m.version = v
}

// SetClaudeDir sets the Claude installation directory path.
func (m *AppModel) SetClaudeDir(dir string) {
	m.claudeDir = dir
}

// SetSyncDir sets the sync repository directory path.
func (m *AppModel) SetSyncDir(dir string) {
	m.syncDir = dir
}

func (m AppModel) Init() tea.Cmd {
	return nil
}

func (m AppModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil
	case tea.KeyMsg:
		// Global keys (work in any view)
		if msg.String() == "q" || msg.String() == "ctrl+c" {
			m.quitting = true
			return m, tea.Quit
		}
		// Route to active view
		switch m.activeView {
		case viewDashboard:
			return m.updateDashboard(msg)
		case viewActions:
			return m.updateActions(msg)
		case viewSubView:
			return m.updateSubView(msg)
		}
	}
	return m, nil
}

func (m AppModel) View() string {
	if m.quitting {
		return ""
	}
	switch m.activeView {
	case viewDashboard:
		return m.viewDashboard()
	case viewActions:
		return m.viewActions()
	case viewSubView:
		return m.viewSubView()
	}
	return ""
}

// --- Dashboard view ---

func (m AppModel) updateDashboard(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		m.activeView = viewActions
	case "j", "down":
		m.dashboardScroll++
	case "k", "up":
		if m.dashboardScroll > 0 {
			m.dashboardScroll--
		}
	}
	return m, nil
}

func (m AppModel) viewDashboard() string {
	return "Dashboard (press enter for actions, q to quit)"
}

// --- Actions view ---

func (m AppModel) updateActions(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.activeView = viewDashboard
	case "j", "down":
		m.actionCursor++
	case "k", "up":
		if m.actionCursor > 0 {
			m.actionCursor--
		}
	}
	return m, nil
}

func (m AppModel) viewActions() string {
	return "Actions (press esc for dashboard, q to quit)"
}

// --- Sub-view ---

func (m AppModel) updateSubView(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.activeView = viewActions
		m.subView = nil
		return m, nil
	}
	if m.subView != nil {
		var cmd tea.Cmd
		m.subView, cmd = m.subView.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m AppModel) viewSubView() string {
	if m.subView != nil {
		return m.subView.View()
	}
	return "Sub-view (press esc to go back)"
}
