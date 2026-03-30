package tui

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/ruminaider/claude-sync/internal/approval"
	"github.com/ruminaider/claude-sync/internal/commands"
)

// approveResultMsg carries the outcome of an approval operation.
type approveResultMsg struct {
	success bool
	message string
	err     error
}

// ApproveView is a scrollable sub-view that displays pending high-risk
// changes and lets the user approve them.
type ApproveView struct {
	pending approval.PendingChanges
	content string // pre-rendered content
	scroll  int
	maxScroll int
	width   int
	height  int

	// Execution state
	executing  bool
	resultDone bool
	resultMsg  string
	resultOk   bool

	// Paths for execution
	claudeDir string
	syncDir   string

	// Error loading pending
	loadErr error
}

// NewApproveView creates a new ApproveView by reading pending changes.
func NewApproveView(width, height int, claudeDir, syncDir string) ApproveView {
	pending, err := approval.ReadPending(syncDir)

	v := ApproveView{
		pending:   pending,
		width:     width,
		height:    height,
		claudeDir: claudeDir,
		syncDir:   syncDir,
		loadErr:   err,
	}

	if err == nil {
		v.content = buildApproveContent(pending)
		v.scroll, v.maxScroll = recalcScroll(v.content, height, 0)
	}

	return v
}

func (m ApproveView) Init() tea.Cmd {
	return nil
}

func (m ApproveView) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		if m.content != "" {
			m.scroll, m.maxScroll = recalcScroll(m.content, m.height, m.scroll)
		}
		return m, nil

	case approveResultMsg:
		m.executing = false
		m.resultDone = true
		m.resultOk = msg.success
		m.resultMsg = resolveResultMsg(msg.success, msg.message, msg.err)
		return m, nil

	case tea.KeyMsg:
		if m.resultDone {
			return m, func() tea.Msg {
				return subViewCloseMsg{refreshState: m.resultOk}
			}
		}

		if m.executing {
			return m, nil
		}

		switch msg.String() {
		case "esc":
			return m, func() tea.Msg {
				return subViewCloseMsg{refreshState: false}
			}
		case "a":
			if m.loadErr != nil || m.pending.IsEmpty() {
				return m, nil
			}
			m.executing = true
			return m, runApprove(m.claudeDir, m.syncDir)
		case "j", "down":
			if m.scroll < m.maxScroll {
				m.scroll++
			}
		case "k", "up":
			if m.scroll > 0 {
				m.scroll--
			}
		}
	}
	return m, nil
}

func (m ApproveView) View() string {
	maxWidth, _ := clampWidth(m.width)
	box := contentBox(maxWidth, colorSurface1)

	if m.loadErr != nil {
		var lines []string
		lines = renderResultLines(lines, false, "Could not read pending changes: "+m.loadErr.Error())
		return box.Render(strings.Join(lines, "\n"))
	}

	if m.resultDone {
		var lines []string
		lines = renderResultLines(lines, m.resultOk, m.resultMsg)
		return box.Render(strings.Join(lines, "\n"))
	}

	if m.executing {
		return box.Render(stYellow.Render("⟳ Applying changes..."))
	}

	if m.pending.IsEmpty() {
		return box.Render(stDim.Render("No pending changes found.") + "\n\n" + stDim.Render("esc back"))
	}

	return renderScrollable(m.content, m.width, m.height, m.scroll, "a approve  j/k scroll  esc back")
}

// runApprove returns a tea.Cmd that executes approval in a goroutine.
func runApprove(claudeDir, syncDir string) tea.Cmd {
	return func() (result tea.Msg) {
		defer func() {
			if r := recover(); r != nil {
				result = approveResultMsg{
					success: false,
					err:     fmt.Errorf("panic: %v", r),
				}
			}
		}()

		approveResult, err := commands.Approve(claudeDir, syncDir)
		if err != nil {
			return approveResultMsg{success: false, err: err}
		}
		return approveResultMsg{
			success: true,
			message: formatApproveResult(approveResult),
		}
	}
}

// buildApproveContent renders pending changes into a human-readable string.
func buildApproveContent(pending approval.PendingChanges) string {
	headerStyle := lipgloss.NewStyle().Bold(true).Foreground(colorText)

	var lines []string
	lines = append(lines, headerStyle.Render("Pending changes for review"))
	lines = append(lines, "")
	lines = append(lines, stDim.Render("These high-risk changes were deferred during pull."))
	lines = append(lines, stDim.Render("They will run commands or modify permissions on your machine."))
	lines = append(lines, "")

	// Permissions
	if pending.Permissions != nil && (len(pending.Permissions.Allow) > 0 || len(pending.Permissions.Deny) > 0) {
		lines = append(lines, sectionLine(stSection, "Permissions", 60))
		for _, rule := range pending.Permissions.Allow {
			lines = append(lines, stGreen.Render("  + allow: "+rule))
		}
		for _, rule := range pending.Permissions.Deny {
			lines = append(lines, stRed.Render("  + deny:  "+rule))
		}
		lines = append(lines, "")
	}

	// MCP Servers
	if len(pending.MCP) > 0 {
		names := sortedJSONKeys(pending.MCP)
		lines = append(lines, sectionLine(stSection, fmt.Sprintf("MCP Servers (%d)", len(names)), 60))
		for _, name := range names {
			lines = append(lines, stText.Render("  + "+name))
			detail := parseMCPDetail(pending.MCP[name])
			if detail != "" {
				lines = append(lines, stDim.Render("    "+detail))
			}
		}
		lines = append(lines, "")
	}

	// Hooks
	if len(pending.Hooks) > 0 {
		hookNames := sortedJSONKeys(pending.Hooks)
		lines = append(lines, sectionLine(stSection, fmt.Sprintf("Hooks (%d)", len(hookNames)), 60))
		for _, name := range hookNames {
			lines = append(lines, stText.Render("  + "+name))
			detail := parseHookDetail(pending.Hooks[name])
			if detail != "" {
				lines = append(lines, stDim.Render("    "+detail))
			}
		}
		lines = append(lines, "")
	}

	return strings.Join(lines, "\n")
}

// parseMCPDetail extracts a human-readable summary from an MCP server JSON config.
func parseMCPDetail(raw json.RawMessage) string {
	var server struct {
		Command string   `json:"command"`
		Args    []string `json:"args"`
		URL     string   `json:"url"`
	}
	if err := json.Unmarshal(raw, &server); err != nil {
		return ""
	}
	if server.Command != "" {
		if len(server.Args) > 0 {
			return "runs: " + server.Command + " " + strings.Join(server.Args, " ")
		}
		return "runs: " + server.Command
	}
	if server.URL != "" {
		return "url: " + server.URL
	}
	return ""
}

// parseHookDetail extracts a human-readable summary from a hook JSON definition.
func parseHookDetail(raw json.RawMessage) string {
	var entries []struct {
		Hooks []struct {
			Type    string `json:"type"`
			Command string `json:"command"`
		} `json:"hooks"`
	}
	if err := json.Unmarshal(raw, &entries); err != nil {
		return ""
	}
	var cmds []string
	for _, entry := range entries {
		for _, h := range entry.Hooks {
			if h.Command != "" {
				cmds = append(cmds, h.Command)
			}
		}
	}
	if len(cmds) == 0 {
		return ""
	}
	if len(cmds) == 1 {
		return "runs: " + cmds[0]
	}
	return "runs: " + strings.Join(cmds, ", ")
}

// sortedJSONKeys returns the keys of a JSON map sorted alphabetically.
func sortedJSONKeys(m map[string]json.RawMessage) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
