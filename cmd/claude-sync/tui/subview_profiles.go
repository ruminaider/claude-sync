package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/ruminaider/claude-sync/internal/commands"
	"github.com/ruminaider/claude-sync/internal/profiles"
)

// subViewCloseMsg signals that a sub-view wants to close.
type subViewCloseMsg struct {
	refreshState bool // true if state should be re-detected
}

// profileSwitchResultMsg carries the outcome of a profile switch operation.
type profileSwitchResultMsg struct {
	success bool
	message string
	err     error
}

// profileOption represents a single profile choice in the picker.
type profileOption struct {
	name        string // "" for base, "work", "personal", etc.
	displayName string // "base (no profile)", "work", etc.
	active      bool   // true if currently active
}

// ProfilePicker is a sub-view for switching the active settings profile.
type ProfilePicker struct {
	profiles      []profileOption
	cursor        int
	width         int
	height        int
	selected      string // set when user selects a profile
	cancelled     bool   // set when user presses Esc
	executing     bool   // true while profile switch is running
	resultMsg     string // success/error message after switch
	resultSuccess bool
	resultDone    bool

	// Paths needed for executing profile switch
	claudeDir string
	syncDir   string
}

// NewProfilePicker creates a ProfilePicker from the current menu state.
func NewProfilePicker(state commands.MenuState, width, height int) ProfilePicker {
	var opts []profileOption

	// Add each named profile first
	for _, name := range state.Profiles {
		opts = append(opts, profileOption{
			name:        name,
			displayName: name,
			active:      name == state.ActiveProfile,
		})
	}

	// Add "base (no profile)" option last
	opts = append(opts, profileOption{
		name:        "",
		displayName: "base (no profile)",
		active:      state.ActiveProfile == "",
	})

	return ProfilePicker{
		profiles: opts,
		width:    width,
		height:   height,
	}
}

// SetPaths sets the Claude and sync directory paths needed for execution.
func (m *ProfilePicker) SetPaths(claudeDir, syncDir string) {
	m.claudeDir = claudeDir
	m.syncDir = syncDir
}

func (m ProfilePicker) Init() tea.Cmd {
	return nil
}

func (m ProfilePicker) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case profileSwitchResultMsg:
		m.executing = false
		m.resultDone = true
		m.resultSuccess = msg.success
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
				return subViewCloseMsg{refreshState: m.resultSuccess}
			}
		}

		// While executing, ignore input
		if m.executing {
			return m, nil
		}

		switch msg.String() {
		case "esc":
			m.cancelled = true
			return m, func() tea.Msg {
				return subViewCloseMsg{refreshState: false}
			}
		case "j", "down":
			if m.cursor < len(m.profiles)-1 {
				m.cursor++
			}
		case "k", "up":
			if m.cursor > 0 {
				m.cursor--
			}
		case "enter":
			opt := m.profiles[m.cursor]
			if opt.active {
				// Already active, show inline message
				m.resultDone = true
				m.resultSuccess = true
				m.resultMsg = "Already active"
				return m, nil
			}
			m.executing = true
			m.selected = opt.name
			return m, switchProfile(opt.name, m.claudeDir, m.syncDir)
		}
	}
	return m, nil
}

func (m ProfilePicker) View() string {
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
	boldBlue := lipgloss.NewStyle().Bold(true).Foreground(colorBlue)
	textStyle := lipgloss.NewStyle().Foreground(colorText)
	yellowStyle := lipgloss.NewStyle().Foreground(colorYellow)

	var lines []string

	lines = append(lines, headerStyle.Render("Switch settings profile"))
	lines = append(lines, "")

	// Result state
	if m.resultDone {
		if m.resultSuccess {
			lines = append(lines, greenStyle.Render("\u2713 "+m.resultMsg))
		} else {
			lines = append(lines, redStyle.Render("\u2717 "+m.resultMsg))
		}
		lines = append(lines, "")
		lines = append(lines, dimStyle.Render("Press any key to go back"))
	} else if m.executing {
		displayName := m.selected
		if displayName == "" {
			displayName = "base config"
		}
		lines = append(lines, yellowStyle.Render("\u27f3 Switching to "+displayName+"..."))
	} else {
		lines = append(lines, dimStyle.Render("Select a profile to activate:"))
		lines = append(lines, "")

		for i, opt := range m.profiles {
			label := opt.displayName
			if opt.active {
				label += " (currently active)"
			}

			if m.cursor == i {
				lines = append(lines, boldBlue.Render("\u25cf "+label))
			} else {
				lines = append(lines, textStyle.Render("  "+label))
			}
		}

		lines = append(lines, "")
		lines = append(lines, dimStyle.Render("Switching profile will auto-pull and apply changes."))
		lines = append(lines, "")
		lines = append(lines, dimStyle.Render("enter select  esc back"))
	}

	content := strings.Join(lines, "\n")

	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorSurface1).
		Padding(1, 2).
		Width(maxWidth)

	return boxStyle.Render(content)
}

// switchProfile returns a tea.Cmd that switches the active profile and auto-pulls.
func switchProfile(name string, claudeDir, syncDir string) tea.Cmd {
	return func() (result tea.Msg) {
		defer func() {
			if r := recover(); r != nil {
				result = profileSwitchResultMsg{
					success: false,
					err:     fmt.Errorf("panic in switchProfile: %v", r),
				}
			}
		}()
		// 1. Set the active profile
		var err error
		if name == "" {
			err = profiles.DeleteActiveProfile(syncDir)
		} else {
			err = profiles.WriteActiveProfile(syncDir, name)
		}
		if err != nil {
			return profileSwitchResultMsg{success: false, err: err}
		}

		// 2. Auto-pull to apply the new profile's settings
		pullResult, pullErr := commands.Pull(claudeDir, syncDir, true)
		if pullErr != nil {
			return profileSwitchResultMsg{
				success: false,
				err:     pullErr,
				message: "Profile set but pull failed: " + pullErr.Error(),
			}
		}

		msg := "Switched to " + name
		if name == "" {
			msg = "Switched to base config"
		}
		if pullResult != nil && len(pullResult.ToInstall) > 0 {
			msg += fmt.Sprintf(", %d plugin(s) updated", len(pullResult.ToInstall))
		}
		return profileSwitchResultMsg{success: true, message: msg}
	}
}
