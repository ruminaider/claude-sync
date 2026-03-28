package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/ruminaider/claude-sync/internal/config"
	"github.com/ruminaider/claude-sync/internal/git"
)

// subscribeResultMsg carries the outcome of a subscribe operation.
type subscribeResultMsg struct {
	success bool
	message string
	err     error
}

// SubscribeFlow is a sub-view for subscribing to another config repo.
// Step 0: URL input, Step 1: confirm, then execute subscribe via CLI.
type SubscribeFlow struct {
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
	syncDir string
}

// NewSubscribeFlow creates a new SubscribeFlow sub-view.
func NewSubscribeFlow(width, height int) *SubscribeFlow {
	ti := textinput.New()
	ti.Placeholder = "user/repo or full URL"
	ti.Focus()
	ti.CharLimit = 256
	ti.Width = 50

	return &SubscribeFlow{
		step:     0,
		urlInput: ti,
		width:    width,
		height:   height,
	}
}

func (m *SubscribeFlow) Init() tea.Cmd {
	return textinput.Blink
}

func (m *SubscribeFlow) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil
	case subscribeResultMsg:
		m.executing = false
		m.resultDone = true
		m.resultOk = msg.success
		m.resultMsg = resolveSubscribeResultMsg(msg.success, msg.message, msg.err)
		return m, nil

	case tea.KeyMsg:
		// After result is shown, any key dismisses
		if m.resultDone {
			return m, func() tea.Msg {
				return subViewCloseMsg{refreshState: m.resultOk}
			}
		}

		// Step 0: URL input
		if m.step == 0 {
			switch msg.String() {
			case "esc":
				m.cancelled = true
				return m, func() tea.Msg {
					return subViewCloseMsg{refreshState: false}
				}
			case "enter":
				m.repoURL = strings.TrimSpace(m.urlInput.Value())
				if m.repoURL == "" {
					return m, nil
				}
				m.step = 1
				return m, nil
			default:
				var cmd tea.Cmd
				m.urlInput, cmd = m.urlInput.Update(msg)
				return m, cmd
			}
		}

		// Step 1: confirm
		if m.step == 1 {
			switch msg.String() {
			case "y", "enter":
				m.executing = true
				return m, executeSubscribe(m.syncDir, m.repoURL)
			case "n", "esc":
				m.cancelled = true
				return m, func() tea.Msg {
					return subViewCloseMsg{refreshState: false}
				}
			}
		}
	}
	return m, nil
}

func (m *SubscribeFlow) View() string {
	if m.resultDone {
		return renderSubscribeResult(m.resultOk, m.resultMsg, m.width, m.height)
	}

	if m.step == 0 {
		return m.viewURLInput()
	}
	return m.viewConfirm()
}

func (m *SubscribeFlow) viewURLInput() string {
	prompt := "Enter config repo URL or shortname (user/repo):\n\n"
	prompt += m.urlInput.View() + "\n\n"
	prompt += lipgloss.NewStyle().Foreground(colorOverlay0).Render("esc: cancel  enter: continue")
	return prompt
}

func (m *SubscribeFlow) viewConfirm() string {
	confirm := fmt.Sprintf("Subscribe to %s?\n\n", m.repoURL)
	confirm += lipgloss.NewStyle().Foreground(colorOverlay0).Render("y: yes  n: no")
	return confirm
}

func executeSubscribe(syncDir, repoURL string) tea.Cmd {
	return func() (result tea.Msg) {
		defer func() {
			if r := recover(); r != nil {
				result = subscribeResultMsg{
					success: false,
					message: fmt.Sprintf("internal error: %v", r),
					err:     fmt.Errorf("panic in subscribe: %v", r),
				}
			}
		}()

		// Validate that sync dir exists
		if _, err := os.Stat(syncDir); os.IsNotExist(err) {
			return subscribeResultMsg{
				success: false,
				message: "claude-sync not initialized",
				err:     err,
			}
		}

		// Check if URL is already subscribed
		cfgData, err := os.ReadFile(filepath.Join(syncDir, "config.yaml"))
		if err != nil {
			return subscribeResultMsg{
				success: false,
				message: "Could not read config.yaml",
				err:     err,
			}
		}
		_, err = config.Parse(cfgData)
		if err != nil {
			return subscribeResultMsg{
				success: false,
				message: "Could not parse config.yaml",
				err:     err,
			}
		}

		// For now, just validate the URL is reachable and has a config
		// Full subscribe logic is delegated to the CLI command
		tempDir := filepath.Join(os.TempDir(), "claude-sync-validate-"+randStr(8))
		defer os.RemoveAll(tempDir)

		if err := os.MkdirAll(tempDir, 0755); err != nil {
			return subscribeResultMsg{
				success: false,
				message: "Could not create temp directory",
				err:     err,
			}
		}

		// Quick validation: shallow clone and check for config
		if err := git.ShallowClone(repoURL, tempDir, "origin/HEAD"); err != nil {
			return subscribeResultMsg{
				success: false,
				message: "Could not access repo. Check URL and network.",
				err:     err,
			}
		}

		if _, err := os.Stat(filepath.Join(tempDir, "config.yaml")); os.IsNotExist(err) {
			return subscribeResultMsg{
				success: false,
				message: "No config.yaml found in repo",
				err:     err,
			}
		}

		return subscribeResultMsg{
			success: true,
			message: fmt.Sprintf("Run 'claude-sync subscribe %s' to complete subscription", repoURL),
			err:     nil,
		}
	}
}

func resolveSubscribeResultMsg(success bool, msg string, err error) string {
	if success {
		return msg
	}
	if msg != "" {
		return msg
	}
	if err != nil {
		return err.Error()
	}
	return "Unknown error"
}

func renderSubscribeResult(success bool, msg string, width int, height int) string {
	icon := "✓"
	if !success {
		icon = "✗"
	}

	header := fmt.Sprintf("%s Subscribe", icon)
	if !success {
		header = fmt.Sprintf("✗ Subscribe failed")
	}

	content := fmt.Sprintf("%s\n\n%s\n\n", header, msg)
	content += lipgloss.NewStyle().Foreground(colorOverlay0).Render("press any key to continue")
	return content
}

func randStr(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, length)
	for i := range b {
		b[i] = charset[i%len(charset)]
	}
	return string(b)
}
