package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ruminaider/claude-sync/internal/commands"
)

// actionStartMsg signals an inline action has started executing.
type actionStartMsg struct {
	itemIndex int
	actionID  string
}

// actionResultMsg signals an inline action has completed.
type actionResultMsg struct {
	itemIndex int
	actionID  string
	success   bool
	message   string
	err       error
}

// executeAction returns a tea.Cmd that runs business logic in a goroutine.
// The result is sent back as an actionResultMsg.
func executeAction(index int, actionID string, args []string,
	claudeDir, syncDir string) tea.Cmd {
	return func() tea.Msg {
		var msg string
		var err error

		switch actionID {
		case "pull":
			result, pullErr := commands.Pull(claudeDir, syncDir, true)
			err = pullErr
			if result != nil {
				msg = formatPullResult(result)
			}
		case "push", "push-changes":
			scanResult, scanErr := commands.PushScan(claudeDir, syncDir)
			if scanErr != nil {
				err = scanErr
			} else if scanResult == nil || !scanResult.HasChanges() {
				msg = "No changes to push"
			} else {
				err = commands.PushApply(commands.PushApplyOptions{
					ClaudeDir: claudeDir,
					SyncDir:   syncDir,
					// TODO: populate from scan result in later task
				})
				if err == nil {
					msg = "Changes pushed successfully"
				}
			}
		case "approve":
			result, approveErr := commands.Approve(claudeDir, syncDir)
			err = approveErr
			if result != nil {
				msg = formatApproveResult(result)
			}
		case "reject":
			err = commands.Reject(syncDir)
			if err == nil {
				msg = "Pending changes rejected"
			}
		case "conflicts":
			// For now, just report -- real conflict resolution needs a sub-view
			msg = "Conflict resolution not yet available in TUI"
		case "plugin-update":
			if len(args) > 0 {
				// TODO: wire to real update command in later task
				msg = "Plugin update not yet available in TUI"
			}
		case "import-mcp":
			// TODO: wire to real MCP import in later task
			msg = "MCP import not yet available in TUI"
		default:
			err = fmt.Errorf("unknown action: %s", actionID)
		}

		return actionResultMsg{
			itemIndex: index,
			actionID:  actionID,
			success:   err == nil,
			message:   msg,
			err:       err,
		}
	}
}

func formatPullResult(r *commands.PullResult) string {
	var parts []string
	installed := len(r.ToInstall)
	removed := len(r.ToRemove)
	if installed > 0 {
		parts = append(parts, fmt.Sprintf("%d plugin(s) installed", installed))
	}
	if removed > 0 {
		parts = append(parts, fmt.Sprintf("%d plugin(s) removed", removed))
	}
	if len(r.SettingsApplied) > 0 {
		parts = append(parts, "settings updated")
	}
	if len(r.PendingHighRisk) > 0 {
		parts = append(parts, "pending high-risk changes need approval")
	}
	if len(parts) == 0 {
		return "Config is up to date"
	}
	return "Pulled \u2014 " + strings.Join(parts, ", ")
}

func formatApproveResult(r *commands.ApproveResult) string {
	var parts []string
	if r.PermissionsApplied {
		parts = append(parts, "permissions applied")
	}
	if len(r.HooksApplied) > 0 {
		parts = append(parts, fmt.Sprintf("%d hook(s) applied", len(r.HooksApplied)))
	}
	if len(r.MCPApplied) > 0 {
		parts = append(parts, fmt.Sprintf("%d MCP server(s) applied", len(r.MCPApplied)))
	}
	if len(parts) == 0 {
		return "Changes approved"
	}
	return "Approved \u2014 " + strings.Join(parts, ", ")
}
