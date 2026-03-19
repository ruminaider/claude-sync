package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/ruminaider/claude-sync/internal/commands"
)

// joinResultMsg carries the outcome of a join operation.
type joinResultMsg struct {
	success bool
	message string
	err     error
}

// JoinFlow is a sub-view for the guided join config flow.
// Step 0: URL input, Step 1: confirm, then execute.
type JoinFlow struct {
	step     int // 0 = URL input, 1 = confirm
	urlInput textinput.Model
	repoURL  string
	width    int
	height   int
	cancelled bool

	// Execution state (after confirm)
	executing  bool
	resultDone bool
	resultMsg  string
	resultOk   bool

	// Paths for execution
	claudeDir string
	syncDir   string
}

// NewJoinFlow creates a new JoinFlow sub-view.
func NewJoinFlow(width, height int) *JoinFlow {
	ti := textinput.New()
	ti.Placeholder = "user/repo"
	ti.Focus()
	ti.CharLimit = 256
	ti.Width = 50

	return &JoinFlow{
		step:     0,
		urlInput: ti,
		width:    width,
		height:   height,
	}
}

func (m *JoinFlow) Init() tea.Cmd {
	return textinput.Blink
}

func (m *JoinFlow) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case joinResultMsg:
		m.executing = false
		m.resultDone = true
		m.resultOk = msg.success
		if msg.success {
			m.resultMsg = msg.message
		} else {
			if msg.message != "" {
				m.resultMsg = msg.message
			} else if msg.err != nil {
				m.resultMsg = msg.err.Error()
			} else {
				m.resultMsg = "Unknown error"
			}
		}
		return m, nil

	case tea.KeyMsg:
		// After result is shown, any key dismisses
		if m.resultDone {
			return m, func() tea.Msg {
				return subViewCloseMsg{refreshState: m.resultOk}
			}
		}

		// While executing, ignore input
		if m.executing {
			return m, nil
		}

		switch msg.String() {
		case "esc":
			if m.step == 1 {
				// Go back to step 0
				m.step = 0
				m.repoURL = ""
				return m, nil
			}
			// Step 0: cancel
			m.cancelled = true
			return m, func() tea.Msg {
				return subViewCloseMsg{refreshState: false}
			}

		case "enter":
			if m.step == 0 {
				url := strings.TrimSpace(m.urlInput.Value())
				if url == "" {
					return m, nil // don't advance on empty URL
				}
				m.repoURL = url
				m.step = 1
				return m, nil
			}
			// Step 1: execute join
			m.executing = true
			return m, executeJoin(m.repoURL, m.claudeDir, m.syncDir)
		}

		// Only pass key messages to text input on step 0
		if m.step == 0 {
			var cmd tea.Cmd
			m.urlInput, cmd = m.urlInput.Update(msg)
			return m, cmd
		}
	}

	return m, nil
}

func (m *JoinFlow) View() string {
	maxWidth := m.width - 2
	if maxWidth > 70 {
		maxWidth = 70
	}
	if maxWidth < 30 {
		maxWidth = 30
	}

	headerStyle := lipgloss.NewStyle().Bold(true).Foreground(colorText)
	dimStyle := lipgloss.NewStyle().Foreground(colorSubtext0)
	greenStyle := lipgloss.NewStyle().Foreground(colorGreen)
	redStyle := lipgloss.NewStyle().Foreground(colorRed)
	yellowStyle := lipgloss.NewStyle().Foreground(colorYellow)
	textStyle := lipgloss.NewStyle().Foreground(colorText)
	boldBlue := lipgloss.NewStyle().Bold(true).Foreground(colorBlue)

	var lines []string

	lines = append(lines, headerStyle.Render("Join a shared config"))
	lines = append(lines, "")

	if m.resultDone {
		if m.resultOk {
			lines = append(lines, greenStyle.Render("\u2713 "+m.resultMsg))
		} else {
			lines = append(lines, redStyle.Render("\u2717 "+m.resultMsg))
		}
		lines = append(lines, "")
		lines = append(lines, dimStyle.Render("Press any key to go back"))
	} else if m.executing {
		lines = append(lines, yellowStyle.Render("\u27f3 Joining "+m.repoURL+"..."))
	} else if m.step == 0 {
		lines = append(lines, textStyle.Render("Enter a config repo URL or GitHub shorthand:"))
		lines = append(lines, "  "+m.urlInput.View())
		lines = append(lines, "")
		lines = append(lines, dimStyle.Render("  Examples:"))
		lines = append(lines, dimStyle.Render("    user/repo"))
		lines = append(lines, dimStyle.Render("    https://github.com/user/repo.git"))
		lines = append(lines, "")
		lines = append(lines, dimStyle.Render("enter continue  esc back"))
	} else if m.step == 1 {
		lines = append(lines, textStyle.Render("Config: ")+boldBlue.Render(m.repoURL))
		lines = append(lines, "")
		lines = append(lines, textStyle.Render("Joining will:"))
		lines = append(lines, dimStyle.Render("  \u2022 Clone the config repo"))
		lines = append(lines, dimStyle.Render("  \u2022 Apply plugins, settings, and hooks"))
		lines = append(lines, dimStyle.Render("  \u2022 High-risk changes will need separate approval"))
		lines = append(lines, "")
		lines = append(lines, boldBlue.Render("> Join and apply this config"))
		lines = append(lines, "")
		lines = append(lines, dimStyle.Render("enter join  esc back"))
	}

	content := strings.Join(lines, "\n")

	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorSurface1).
		Padding(1, 2).
		Width(maxWidth)

	return boxStyle.Render(content)
}

// executeJoin returns a tea.Cmd that runs the join operation.
func executeJoin(repoURL, claudeDir, syncDir string) tea.Cmd {
	return func() tea.Msg {
		result, err := commands.Join(repoURL, claudeDir, syncDir)
		if err != nil {
			return joinResultMsg{success: false, err: err}
		}

		msg := "Joined successfully"
		if result != nil {
			var details []string
			if result.HasSettings {
				details = append(details, "settings applied")
			}
			if result.HasHooks {
				details = append(details, "hooks applied")
			}
			if result.HasProfiles {
				details = append(details, fmt.Sprintf("%d profiles available", len(result.ProfileNames)))
			}
			if len(details) > 0 {
				msg += ", " + strings.Join(details, ", ")
			}
		}
		return joinResultMsg{success: true, message: msg}
	}
}
