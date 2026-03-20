package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/ruminaider/claude-sync/internal/commands"
)

type appView int

const (
	viewMain appView = iota
	viewSubView
)

// AppModel is the top-level Bubble Tea model that routes between
// the main screen and sub-view screens via a simple state machine.
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
	actionCursor       int
	freshInstallCursor int // 0 = Create, 1 = Join (only used when !ConfigExists)

	// Action screen state
	recommendations []recommendation
	intents         []intent

	// Filter mode state
	filterMode bool
	filterText string

	// Help overlay state
	showHelp bool

	// Inline action execution state
	executing        bool                   // true while action is running
	executingIndex   int                    // which item is executing
	executionResults map[int]actionResultMsg // results keyed by item index

	// sub-view state (populated when activeView == viewSubView)
	subView tea.Model

	// Exit signals for the caller
	LaunchConfigEditor bool // true = caller should run config editor, then re-launch AppModel
}

// NewAppModel creates an AppModel from detected state, starting on the main screen.
func NewAppModel(state commands.MenuState) AppModel {
	m := AppModel{
		state:            state,
		activeView:       viewMain,
		executionResults: make(map[int]actionResultMsg),
	}
	// Build recommendations and intents immediately so they're available on first render
	if state.ConfigExists {
		m.recommendations = buildRecommendations(state)
		m.intents = buildIntents(state)
	}
	return m
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
		m.activeView = viewMain
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
	case joinResultMsg:
		// Route to sub-view
		if m.subView != nil {
			var cmd tea.Cmd
			m.subView, cmd = m.subView.Update(msg)
			return m, cmd
		}
		return m, nil
	case tea.KeyMsg:
		// Global keys (work in any view unless filter/help is active)
		if msg.String() == "ctrl+c" {
			m.quitting = true
			return m, tea.Quit
		}
		if msg.String() == "q" && m.activeView == viewMain && !m.filterMode && !m.showHelp {
			m.quitting = true
			return m, tea.Quit
		}
		// Route to active view
		switch m.activeView {
		case viewMain:
			return m.updateMain(msg)
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
	case viewMain:
		return m.viewMain()
	case viewSubView:
		return m.viewSubView()
	}
	return ""
}

// --- Main view ---

func (m AppModel) updateMain(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if !m.state.ConfigExists {
		// Fresh install mode: navigate between Create and Join
		switch msg.String() {
		case "up", "k":
			if m.freshInstallCursor > 0 {
				m.freshInstallCursor--
			}
		case "down", "j":
			if m.freshInstallCursor < 1 {
				m.freshInstallCursor++
			}
		case "enter":
			if m.freshInstallCursor == 0 {
				// Create new config → launch config editor
				m.LaunchConfigEditor = true
				return m, tea.Quit
			}
			// Join shared config → launch JoinFlow
			jf := NewJoinFlow(m.width, m.height)
			jf.claudeDir = m.claudeDir
			jf.syncDir = m.syncDir
			m.subView = jf
			m.activeView = viewSubView
			return m, jf.Init()
		}
		return m, nil
	}

	// Help overlay takes priority — any key dismisses it
	if m.showHelp {
		m.showHelp = false
		return m, nil
	}

	// Filter mode — route keystrokes to filter input
	if m.filterMode {
		return m.updateFilterMode(msg)
	}

	// Get filtered lists for navigation
	filteredRecs := filterRecommendations(m.recommendations, m.filterText)
	filteredIntents := filterIntents(m.intents, m.filterText)
	total := actionItemCount(filteredRecs, filteredIntents)

	switch msg.String() {
	case "/":
		m.filterMode = true
		m.filterText = ""
		m.actionCursor = 0
		return m, nil
	case "?":
		m.showHelp = true
		return m, nil
	case "esc":
		if m.filterText != "" {
			m.filterText = ""
			m.actionCursor = 0
			return m, nil
		}
		// Esc from main screen quits (no dashboard to go back to)
		m.quitting = true
		return m, tea.Quit
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
		action := selectedAction(filteredRecs, filteredIntents, m.actionCursor)
		if action == nil {
			return m, nil
		}
		if action.inline {
			// Execute inline — find the real index in the unfiltered list for result tracking
			m.executing = true
			m.executingIndex = m.actionCursor
			return m, executeAction(m.actionCursor, action.id, action.args, m.claudeDir, m.syncDir)
		}
		// Sub-view navigation
		return m.openSubView(action.id)
	}
	return m, nil
}

func (m AppModel) updateFilterMode(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEscape:
		m.filterMode = false
		m.filterText = ""
		m.actionCursor = 0
		return m, nil
	case tea.KeyBackspace:
		if len(m.filterText) > 0 {
			m.filterText = m.filterText[:len(m.filterText)-1]
			m.actionCursor = 0
		}
		return m, nil
	case tea.KeyEnter:
		m.filterMode = false
		// Execute the selected item from filtered list
		filteredRecs := filterRecommendations(m.recommendations, m.filterText)
		filteredIntents := filterIntents(m.intents, m.filterText)
		action := selectedAction(filteredRecs, filteredIntents, m.actionCursor)
		if action == nil {
			return m, nil
		}
		if m.executing {
			return m, nil
		}
		if action.inline {
			m.executing = true
			m.executingIndex = m.actionCursor
			return m, executeAction(m.actionCursor, action.id, action.args, m.claudeDir, m.syncDir)
		}
		return m.openSubView(action.id)
	case tea.KeyRunes:
		m.filterText += string(msg.Runes)
		m.actionCursor = 0
		return m, nil
	}

	// Arrow key navigation within filter mode (j/k are typed as filter text)
	switch msg.Type {
	case tea.KeyDown:
		filteredRecs := filterRecommendations(m.recommendations, m.filterText)
		filteredIntents := filterIntents(m.intents, m.filterText)
		total := actionItemCount(filteredRecs, filteredIntents)
		if m.actionCursor < total-1 {
			m.actionCursor++
		}
	case tea.KeyUp:
		if m.actionCursor > 0 {
			m.actionCursor--
		}
	}
	return m, nil
}

// openSubView handles navigation to a sub-view based on action ID.
func (m AppModel) openSubView(actionID string) (tea.Model, tea.Cmd) {
	switch actionID {
	case "switch-profile":
		picker := NewProfilePicker(m.state, m.width, m.height)
		picker.SetPaths(m.claudeDir, m.syncDir)
		m.subView = picker
		m.activeView = viewSubView
	case "browse-plugins":
		browser := NewPluginBrowser(m.state, m.width, m.height)
		m.subView = browser
		m.activeView = viewSubView
	case "join-config":
		jf := NewJoinFlow(m.width, m.height)
		jf.claudeDir = m.claudeDir
		jf.syncDir = m.syncDir
		m.subView = jf
		m.activeView = viewSubView
	case "view-config":
		m.subView = NewConfigDetails(m.state, m.width, m.height)
		m.activeView = viewSubView
	case "view-plugins":
		m.subView = NewActivePluginsView(m.state, m.width, m.height)
		m.activeView = viewSubView
	case "edit-config":
		m.LaunchConfigEditor = true
		return m, tea.Quit
	}
	if m.subView != nil {
		return m, m.subView.Init()
	}
	return m, nil
}

func (m AppModel) viewMain() string {
	if !m.state.ConfigExists {
		return renderFreshInstall(m.width, m.height, m.version, m.freshInstallCursor)
	}

	if m.showHelp {
		return m.viewMainHelp()
	}

	recs := filterRecommendations(m.recommendations, m.filterText)
	intents := filterIntents(m.intents, m.filterText)

	return renderMainScreen(m.state, recs, intents,
		m.actionCursor, m.width, m.height, m.version,
		m.executing, m.executingIndex, m.executionResults,
		m.filterMode, m.filterText)
}

func (m AppModel) viewMainHelp() string {
	dimStyle := lipgloss.NewStyle().Foreground(colorSubtext0)
	headerStyle := lipgloss.NewStyle().Bold(true).Foreground(colorText)
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(colorBlue)

	maxWidth := m.width - 2
	if maxWidth > 70 {
		maxWidth = 70
	}
	if maxWidth < 30 {
		maxWidth = 30
	}

	var lines []string
	lines = append(lines, titleStyle.Render("Help"))
	lines = append(lines, "")
	lines = append(lines, headerStyle.Render("Navigation"))
	lines = append(lines, dimStyle.Render("  "+lipgloss.NewStyle().Foreground(colorText).Render("\u2191/k")+"     Move up"))
	lines = append(lines, dimStyle.Render("  "+lipgloss.NewStyle().Foreground(colorText).Render("\u2193/j")+"     Move down"))
	lines = append(lines, dimStyle.Render("  "+lipgloss.NewStyle().Foreground(colorText).Render("enter")+"   Execute action or open sub-view"))
	lines = append(lines, dimStyle.Render("  "+lipgloss.NewStyle().Foreground(colorText).Render("esc")+"     Quit"))
	lines = append(lines, "")
	lines = append(lines, headerStyle.Render("Actions"))
	lines = append(lines, dimStyle.Render("  "+lipgloss.NewStyle().Foreground(colorText).Render("/")+"       Filter actions"))
	lines = append(lines, dimStyle.Render("  "+lipgloss.NewStyle().Foreground(colorText).Render("?")+"       Show this help"))
	lines = append(lines, dimStyle.Render("  "+lipgloss.NewStyle().Foreground(colorText).Render("q")+"       Quit"))
	lines = append(lines, "")
	lines = append(lines, dimStyle.Render("press any key to close"))

	content := strings.Join(lines, "\n")

	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorBlue).
		Padding(1, 2).
		Width(maxWidth)

	return boxStyle.Render(content)
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
		m.activeView = viewMain
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
