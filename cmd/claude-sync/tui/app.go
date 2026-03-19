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

	// Action screen state
	recommendations []recommendation
	intents         []intent

	// Inline action execution state
	executing        bool                   // true while action is running
	executingIndex   int                    // which item is executing
	executionResults map[int]actionResultMsg // results keyed by item index

	// sub-view state (populated when activeView == viewSubView)
	subView tea.Model
}

// NewAppModel creates an AppModel from detected state, starting on the dashboard.
func NewAppModel(state commands.MenuState) AppModel {
	return AppModel{
		state:            state,
		activeView:       viewDashboard,
		executionResults: make(map[int]actionResultMsg),
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
	case actionStartMsg:
		m.executing = true
		m.executingIndex = msg.itemIndex
		return m, nil
	case actionResultMsg:
		m.executing = false
		if m.executionResults == nil {
			m.executionResults = make(map[int]actionResultMsg)
		}
		m.executionResults[msg.itemIndex] = msg
		// Re-detect state and rebuild recommendations
		m.state = commands.DetectMenuState(m.claudeDir, m.syncDir)
		m.recommendations = buildRecommendations(m.state)
		m.intents = buildIntents(m.state)
		return m, nil
	case subViewCloseMsg:
		m.activeView = viewActions
		m.subView = nil
		if msg.refreshState {
			m.state = commands.DetectMenuState(m.claudeDir, m.syncDir)
			m.recommendations = buildRecommendations(m.state)
			m.intents = buildIntents(m.state)
		}
		return m, nil
	case profileSwitchResultMsg:
		// Route to sub-view
		if m.subView != nil {
			var cmd tea.Cmd
			m.subView, cmd = m.subView.Update(msg)
			return m, cmd
		}
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
		m.recommendations = buildRecommendations(m.state)
		m.intents = buildIntents(m.state)
		m.actionCursor = 0
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
	return renderDashboard(m.state, m.width, m.height, m.version)
}

// --- Actions view ---

func (m AppModel) updateActions(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	total := actionItemCount(m.recommendations, m.intents)
	switch msg.String() {
	case "esc":
		m.activeView = viewDashboard
	case "j", "down":
		if m.actionCursor < total-1 {
			m.actionCursor++
		}
	case "k", "up":
		if m.actionCursor > 0 {
			m.actionCursor--
		}
	case "enter":
		if m.executing {
			return m, nil // ignore while executing
		}
		action := selectedAction(m.recommendations, m.intents, m.actionCursor)
		if action == nil {
			return m, nil
		}
		if action.inline {
			// Execute inline
			m.executing = true
			m.executingIndex = m.actionCursor
			return m, executeAction(m.actionCursor, action.id, action.args, m.claudeDir, m.syncDir)
		}
		// Sub-view navigation
		switch action.id {
		case "switch-profile":
			picker := NewProfilePicker(m.state, m.width, m.height)
			picker.SetPaths(m.claudeDir, m.syncDir)
			m.subView = picker
			m.activeView = viewSubView
		}
		// Other sub-views wired in Tasks 8-10
	}
	return m, nil
}

func (m AppModel) viewActions() string {
	return renderActionsWithState(m.recommendations, m.intents, m.actionCursor, m.width, m.height, m.executing, m.executingIndex, m.executionResults)
}

// --- Sub-view ---

func (m AppModel) updateSubView(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.subView != nil {
		var cmd tea.Cmd
		m.subView, cmd = m.subView.Update(msg)
		return m, cmd
	}
	// Fallback: no sub-view loaded, esc goes back
	if msg.String() == "esc" {
		m.activeView = viewActions
		return m, nil
	}
	return m, nil
}

func (m AppModel) viewSubView() string {
	if m.subView != nil {
		return m.subView.View()
	}
	return "Sub-view (press esc to go back)"
}
