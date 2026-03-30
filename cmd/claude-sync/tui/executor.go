package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ruminaider/claude-sync/internal/commands"
	"github.com/ruminaider/claude-sync/internal/git"
)

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
	return func() (result tea.Msg) {
		defer func() {
			if r := recover(); r != nil {
				result = actionResultMsg{
					actionID:  actionID,
					itemIndex: index,
					success:   false,
					message:   fmt.Sprintf("internal error: %v", r),
					err:       fmt.Errorf("panic in %s: %v", actionID, r),
				}
			}
		}()

		var msg string
		var err error

		switch actionID {
		case ActionPull:
			// Fetch once so both preview and pull share the same refs.
			var fetchWarning string
			if git.HasRemote(syncDir, "origin") {
				if fetchErr := git.Fetch(syncDir); fetchErr != nil {
					fetchWarning = fmt.Sprintf("fetch failed (%v); results may be stale", fetchErr)
				}
			}
			// Show preview summary of incoming changes before applying.
			// Even when preview reports nothing to change, still run pull
			// for local-only reconciliation (settings, plugins, etc.).
			preview, previewErr := commands.PullPreview(syncDir)
			result, pullErr := commands.PullWithOptions(commands.PullOptions{
				ClaudeDir: claudeDir,
				SyncDir:   syncDir,
				Quiet:     true,
				SkipFetch: true, // already fetched above
			})
			err = pullErr
			if result != nil {
				pullMsg := formatPullResult(result)
				// Prepend preview context when there were remote changes.
				if previewErr != nil {
					pullMsg = "Preview unavailable: " + previewErr.Error() + ". " + pullMsg
				} else if !preview.NothingToChange {
					pullMsg = fmt.Sprintf("Fetched %d commit(s) \u2014 %s", preview.CommitsBehind, pullMsg)
				}
				if fetchWarning != "" {
					pullMsg = "Warning: " + fetchWarning + ". " + pullMsg
				}
				msg = pullMsg
			}
		case ActionPush, ActionPushChanges:
			scanResult, scanErr := commands.PushScan(claudeDir, syncDir)
			if scanErr != nil {
				err = scanErr
			} else if scanResult == nil || !scanResult.HasChanges() {
				msg = "No changes to push"
			} else {
				err = commands.PushApply(commands.PushApplyOptions{
					ClaudeDir:         claudeDir,
					SyncDir:           syncDir,
					AddPlugins:        scanResult.AddedPlugins,
					RemovePlugins:     scanResult.RemovedPlugins,
					UpdatePermissions: scanResult.ChangedPermissions,
					UpdateClaudeMD:    scanResult.ChangedClaudeMD != nil,
					UpdateMCP:         scanResult.ChangedMCP,
					UpdateKeybindings: scanResult.ChangedKeybindings,
					UpdateCommands:    scanResult.ChangedCommands,
					UpdateSkills:      scanResult.ChangedSkills,
					OrphanedCommands:  scanResult.OrphanedCommands,
					OrphanedSkills:    scanResult.OrphanedSkills,
					DirtyWorkingTree:  scanResult.DirtyWorkingTree,
				})
				if err == nil {
					msg = fmt.Sprintf("Pushed \u2014 %s", commands.PushPreviewSummary(scanResult))
				}
			}
		case ActionConflicts:
			// For now, just report -- real conflict resolution needs a sub-view
			msg = "Conflict resolution not yet available in TUI"
		case ActionPluginUpdate:
			if len(args) > 0 {
				msg = fmt.Sprintf("Plugin update for %s not yet available in TUI", args[0])
			} else {
				msg = "Plugin update not yet available in TUI"
			}
		case ActionImportMCP:
			// TODO: wire to real MCP import in later task
			msg = "MCP import not yet available in TUI"
		case ActionRemovePlugin:
			msg = "Plugin removal not yet available in TUI — use 'claude-sync config update'"
		case ActionForkPlugin:
			msg = "Fork not yet available in TUI — use 'claude-sync fork <plugin>'"
		case ActionPinPlugin:
			msg = "Pin not yet available in TUI — use 'claude-sync pin <plugin> <version>'"
		case "__test_panic":
			panic("test panic")
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
