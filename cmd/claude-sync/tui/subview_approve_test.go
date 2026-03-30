package tui

import (
	"encoding/json"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ruminaider/claude-sync/internal/approval"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewApproveView_ReadsPending(t *testing.T) {
	claudeDir := t.TempDir()
	syncDir := t.TempDir()
	pending := approval.PendingChanges{
		Commit: "abc123",
		Permissions: &approval.PendingPermissions{
			Allow: []string{"Bash(git *)"},
		},
	}
	require.NoError(t, approval.WritePending(syncDir, pending))

	v := NewApproveView(80, 24, claudeDir, syncDir)
	assert.Nil(t, v.loadErr)
	assert.False(t, v.pending.IsEmpty())
	assert.NotEmpty(t, v.content)
}

func TestNewApproveView_EmptyPending(t *testing.T) {
	claudeDir := t.TempDir()
	syncDir := t.TempDir()
	v := NewApproveView(80, 24, claudeDir, syncDir)
	assert.Nil(t, v.loadErr)
	assert.True(t, v.pending.IsEmpty())
}

func TestApproveView_EscCloses(t *testing.T) {
	claudeDir := t.TempDir()
	syncDir := t.TempDir()
	pending := approval.PendingChanges{
		Commit:      "abc",
		Permissions: &approval.PendingPermissions{Allow: []string{"Bash"}},
	}
	require.NoError(t, approval.WritePending(syncDir, pending))

	v := NewApproveView(80, 24, claudeDir, syncDir)
	_, cmd := v.Update(tea.KeyMsg{Type: tea.KeyEscape})
	require.NotNil(t, cmd)
	msg := cmd()
	closeMsg, ok := msg.(subViewCloseMsg)
	require.True(t, ok)
	assert.False(t, closeMsg.refreshState)
}

func TestApproveView_ApproveKey_StartsExecution(t *testing.T) {
	claudeDir := t.TempDir()
	syncDir := t.TempDir()
	pending := approval.PendingChanges{
		Commit:      "abc",
		Permissions: &approval.PendingPermissions{Allow: []string{"Bash"}},
	}
	require.NoError(t, approval.WritePending(syncDir, pending))

	v := NewApproveView(80, 24, claudeDir, syncDir)
	updated, cmd := v.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	av := updated.(ApproveView)
	assert.True(t, av.executing)
	assert.NotNil(t, cmd)
}

func TestApproveView_ApproveKey_IgnoredWhenEmpty(t *testing.T) {
	claudeDir := t.TempDir()
	syncDir := t.TempDir()
	v := NewApproveView(80, 24, claudeDir, syncDir)
	updated, cmd := v.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	av := updated.(ApproveView)
	assert.False(t, av.executing)
	assert.Nil(t, cmd)
}

func TestApproveView_IgnoresInputWhileExecuting(t *testing.T) {
	claudeDir := t.TempDir()
	syncDir := t.TempDir()
	pending := approval.PendingChanges{
		Commit:      "abc",
		Permissions: &approval.PendingPermissions{Allow: []string{"Bash"}},
	}
	require.NoError(t, approval.WritePending(syncDir, pending))

	v := NewApproveView(80, 24, claudeDir, syncDir)
	v.executing = true
	_, cmd := v.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	assert.Nil(t, cmd)
}

func TestApproveView_ResultDismisses(t *testing.T) {
	claudeDir := t.TempDir()
	syncDir := t.TempDir()
	v := NewApproveView(80, 24, claudeDir, syncDir)
	v.resultDone = true
	v.resultOk = true

	_, cmd := v.Update(tea.KeyMsg{Type: tea.KeyEnter})
	require.NotNil(t, cmd)
	msg := cmd()
	closeMsg, ok := msg.(subViewCloseMsg)
	require.True(t, ok)
	assert.True(t, closeMsg.refreshState)
}

func TestApproveView_ResultMsg_SetsState(t *testing.T) {
	claudeDir := t.TempDir()
	syncDir := t.TempDir()
	v := NewApproveView(80, 24, claudeDir, syncDir)
	v.executing = true

	updated, _ := v.Update(approveResultMsg{
		success: true,
		message: "Approved: permissions applied",
	})
	av := updated.(ApproveView)
	assert.False(t, av.executing)
	assert.True(t, av.resultDone)
	assert.True(t, av.resultOk)
	assert.Equal(t, "Approved: permissions applied", av.resultMsg)
}

func TestApproveView_Scroll(t *testing.T) {
	claudeDir := t.TempDir()
	syncDir := t.TempDir()
	pending := approval.PendingChanges{
		Commit:      "abc",
		Permissions: &approval.PendingPermissions{Allow: []string{"A", "B", "C"}},
		MCP:         map[string]json.RawMessage{"server": json.RawMessage(`{"command":"npx"}`)},
		Hooks:       map[string]json.RawMessage{"PreToolUse": json.RawMessage(`[{"hooks":[{"type":"command","command":"validate"}]}]`)},
	}
	require.NoError(t, approval.WritePending(syncDir, pending))

	v := NewApproveView(80, 10, claudeDir, syncDir) // small height to force scroll
	initial := v.scroll

	updated, _ := v.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	av := updated.(ApproveView)
	if av.maxScroll > 0 {
		assert.Greater(t, av.scroll, initial)
	}
}

func TestBuildApproveContent_AllSections(t *testing.T) {
	pending := approval.PendingChanges{
		Permissions: &approval.PendingPermissions{
			Allow: []string{"Bash(git *)"},
			Deny:  []string{"rm -rf"},
		},
		MCP: map[string]json.RawMessage{
			"memory": json.RawMessage(`{"command":"npx","args":["@modelcontextprotocol/server-memory"]}`),
		},
		Hooks: map[string]json.RawMessage{
			"PreToolUse": json.RawMessage(`[{"hooks":[{"type":"command","command":"validate-tool"}]}]`),
		},
	}

	content := buildApproveContent(pending)
	assert.Contains(t, content, "Permissions")
	assert.Contains(t, content, "Bash(git *)")
	assert.Contains(t, content, "rm -rf")
	assert.Contains(t, content, "MCP Servers")
	assert.Contains(t, content, "memory")
	assert.Contains(t, content, "npx")
	assert.Contains(t, content, "Hooks")
	assert.Contains(t, content, "PreToolUse")
	assert.Contains(t, content, "validate-tool")
}

func TestParseMCPDetail_CommandWithArgs(t *testing.T) {
	raw := json.RawMessage(`{"command":"npx","args":["@modelcontextprotocol/server-memory"]}`)
	detail := parseMCPDetail(raw)
	assert.Equal(t, "runs: npx @modelcontextprotocol/server-memory", detail)
}

func TestParseMCPDetail_CommandOnly(t *testing.T) {
	raw := json.RawMessage(`{"command":"my-server"}`)
	detail := parseMCPDetail(raw)
	assert.Equal(t, "runs: my-server", detail)
}

func TestParseMCPDetail_URL(t *testing.T) {
	raw := json.RawMessage(`{"url":"https://mcp.example.com"}`)
	detail := parseMCPDetail(raw)
	assert.Equal(t, "url: https://mcp.example.com", detail)
}

func TestParseMCPDetail_InvalidJSON(t *testing.T) {
	raw := json.RawMessage(`{invalid`)
	detail := parseMCPDetail(raw)
	assert.Equal(t, "", detail)
}

func TestParseHookDetail_SingleCommand(t *testing.T) {
	raw := json.RawMessage(`[{"hooks":[{"type":"command","command":"validate-tool"}]}]`)
	detail := parseHookDetail(raw)
	assert.Equal(t, "runs: validate-tool", detail)
}

func TestParseHookDetail_MultipleCommands(t *testing.T) {
	raw := json.RawMessage(`[{"hooks":[{"type":"command","command":"check-a"},{"type":"command","command":"check-b"}]}]`)
	detail := parseHookDetail(raw)
	assert.Equal(t, "runs: check-a, check-b", detail)
}

func TestParseHookDetail_InvalidJSON(t *testing.T) {
	raw := json.RawMessage(`{invalid`)
	detail := parseHookDetail(raw)
	assert.Equal(t, "", detail)
}

func TestApproveView_WindowSizeMsg(t *testing.T) {
	claudeDir := t.TempDir()
	syncDir := t.TempDir()
	v := NewApproveView(80, 24, claudeDir, syncDir)
	updated, _ := v.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	av := updated.(ApproveView)
	assert.Equal(t, 100, av.width)
	assert.Equal(t, 30, av.height)
}
