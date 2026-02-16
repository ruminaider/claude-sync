package commands_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/ruminaider/claude-sync/internal/approval"
	"github.com/ruminaider/claude-sync/internal/claudecode"
	"github.com/ruminaider/claude-sync/internal/commands"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupApproveEnv(t *testing.T) (claudeDir, syncDir string) {
	t.Helper()
	claudeDir = t.TempDir()
	syncDir = t.TempDir()

	// Create minimal Claude directory structure.
	os.MkdirAll(filepath.Join(claudeDir, "plugins"), 0755)

	// Write minimal settings.json.
	settings := map[string]json.RawMessage{}
	claudecode.WriteSettings(claudeDir, settings)

	return claudeDir, syncDir
}

func TestApproveAppliesPermissions(t *testing.T) {
	claudeDir, syncDir := setupApproveEnv(t)

	pending := approval.PendingChanges{
		Commit: "abc123",
		Permissions: &approval.PendingPermissions{
			Allow: []string{"Bash(git *)"},
			Deny:  []string{"Bash(rm -rf *)"},
		},
	}
	require.NoError(t, approval.WritePending(syncDir, pending))

	result, err := commands.Approve(claudeDir, syncDir)
	require.NoError(t, err)
	assert.True(t, result.PermissionsApplied)

	// Verify permissions were written to settings.json.
	settings, err := claudecode.ReadSettings(claudeDir)
	require.NoError(t, err)

	var perms struct {
		Allow []string `json:"allow"`
		Deny  []string `json:"deny"`
	}
	require.NoError(t, json.Unmarshal(settings["permissions"], &perms))
	assert.Contains(t, perms.Allow, "Bash(git *)")
	assert.Contains(t, perms.Deny, "Bash(rm -rf *)")

	// Verify pending file was cleared.
	p, err := approval.ReadPending(syncDir)
	require.NoError(t, err)
	assert.True(t, p.IsEmpty())
}

func TestApproveAppliesMCP(t *testing.T) {
	claudeDir, syncDir := setupApproveEnv(t)

	mcpData, _ := json.Marshal(map[string]any{
		"command": "npx",
		"args":    []string{"-y", "my-server"},
	})

	pending := approval.PendingChanges{
		Commit: "abc123",
		MCP: map[string]json.RawMessage{
			"my-server": json.RawMessage(mcpData),
		},
	}
	require.NoError(t, approval.WritePending(syncDir, pending))

	result, err := commands.Approve(claudeDir, syncDir)
	require.NoError(t, err)
	assert.Contains(t, result.MCPApplied, "my-server")

	// Verify MCP config was written.
	mcp, err := claudecode.ReadMCPConfig(claudeDir)
	require.NoError(t, err)
	assert.Contains(t, mcp, "my-server")
}

func TestApproveAppliesHooks(t *testing.T) {
	claudeDir, syncDir := setupApproveEnv(t)

	hookData, _ := json.Marshal([]map[string]any{
		{"matcher": "", "hooks": []map[string]any{
			{"type": "command", "command": "echo hello"},
		}},
	})

	pending := approval.PendingChanges{
		Commit: "abc123",
		Hooks: map[string]json.RawMessage{
			"PreCompact": json.RawMessage(hookData),
		},
	}
	require.NoError(t, approval.WritePending(syncDir, pending))

	result, err := commands.Approve(claudeDir, syncDir)
	require.NoError(t, err)
	assert.Contains(t, result.HooksApplied, "PreCompact")

	// Verify hooks were merged into settings.json.
	settings, err := claudecode.ReadSettings(claudeDir)
	require.NoError(t, err)

	var hooks map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(settings["hooks"], &hooks))
	assert.Contains(t, hooks, "PreCompact")
}

func TestApproveNoPending(t *testing.T) {
	claudeDir, syncDir := setupApproveEnv(t)

	_, err := commands.Approve(claudeDir, syncDir)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no pending changes to approve")
}

func TestApproveClears(t *testing.T) {
	claudeDir, syncDir := setupApproveEnv(t)

	pending := approval.PendingChanges{
		Commit: "abc123",
		Permissions: &approval.PendingPermissions{
			Allow: []string{"Bash(git *)"},
		},
	}
	require.NoError(t, approval.WritePending(syncDir, pending))

	// Verify file exists before approve.
	_, err := os.Stat(filepath.Join(syncDir, "pending-changes.yaml"))
	require.NoError(t, err)

	_, err = commands.Approve(claudeDir, syncDir)
	require.NoError(t, err)

	// Verify file is gone after approve.
	_, err = os.Stat(filepath.Join(syncDir, "pending-changes.yaml"))
	assert.True(t, os.IsNotExist(err))
}
