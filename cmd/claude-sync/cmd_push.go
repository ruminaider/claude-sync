package main

import (
	"errors"
	"fmt"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/ruminaider/claude-sync/internal/commands"
	"github.com/ruminaider/claude-sync/internal/git"
	"github.com/ruminaider/claude-sync/internal/paths"
	"github.com/ruminaider/claude-sync/internal/profiles"
	"github.com/spf13/cobra"
)

var pushMessage string
var pushAll bool
var pushRemote string
var pushAutoFlag bool
var pushQuietFlag bool
var pushForceFlag bool

var pushCmd = &cobra.Command{
	Use:   "push",
	Short: "Push changes to remote",
	RunE: func(cmd *cobra.Command, args []string) error {
		claudeDir := paths.ClaudeDir()
		syncDir := paths.SyncDir()

		// Ensure a remote is configured before doing any work.
		if err := ensureRemote(syncDir); err != nil {
			return err
		}

		// Fetch remote refs so unpushed-commit detection is accurate.
		if git.HasRemote(syncDir, "origin") {
			git.FetchPrune(syncDir)
		}

		fmt.Println("Scanning local state...")
		scan, err := commands.PushScan(claudeDir, syncDir)
		if err != nil {
			return err
		}

		if !scan.HasChanges() {
			return pushUnpushedOrNoop(syncDir, pushForceFlag)
		}

		if pushAutoFlag {
			headBefore, _ := git.RevParse(syncDir, "HEAD")
			err := commands.PushApply(commands.PushApplyOptions{
				ClaudeDir:         claudeDir,
				SyncDir:           syncDir,
				AddPlugins:        scan.AddedPlugins,
				RemovePlugins:     scan.RemovedPlugins,
				UpdatePermissions: scan.ChangedPermissions,
				UpdateClaudeMD:    scan.ChangedClaudeMD != nil,
				UpdateMCP:         scan.ChangedMCP,
				UpdateKeybindings: scan.ChangedKeybindings,
				UpdateCommands:    scan.ChangedCommands,
				UpdateSkills:      scan.ChangedSkills,
				Message:           pushMessage,
				Force:             pushForceFlag,
			})
			if err != nil {
				var nffErr *git.NonFastForwardError
				if errors.As(err, &nffErr) {
					if handleErr := handlePushRejection(syncDir); handleErr != nil {
						resetIfNewCommit(syncDir, headBefore)
						return handleErr
					}
					if !pushQuietFlag {
						fmt.Println("Changes pushed successfully.")
					}
					return nil
				}
				resetIfNewCommit(syncDir, headBefore)
				return err
			}
			if !pushQuietFlag {
				fmt.Println("Changes pushed successfully.")
			}
			return nil
		}

		// Non-plugin changes are always included — they reflect local
		// edits the user already made.
		updatePerms := scan.ChangedPermissions
		updateClaudeMD := scan.ChangedClaudeMD != nil
		updateMCP := scan.ChangedMCP
		updateKB := scan.ChangedKeybindings
		updateCmds := scan.ChangedCommands
		updateSkills := scan.ChangedSkills
		hasNonPluginChanges := updatePerms || updateClaudeMD || updateMCP || updateKB || updateCmds || updateSkills

		var selectedAdd []string
		var selectedRemove []string
		var unselectedPlugins []string
		var profileTarget string

		if pushAll {
			selectedAdd = scan.AddedPlugins
			selectedRemove = scan.RemovedPlugins
		} else {
			if len(scan.AddedPlugins) > 0 {
				selected, err := runPicker("Untracked plugins:", scan.AddedPlugins)
				if err != nil {
					return err
				}
				selectedAdd = selected

				// Compute unselected plugins for exclusion.
				selectedSet := make(map[string]bool, len(selectedAdd))
				for _, s := range selectedAdd {
					selectedSet[s] = true
				}
				for _, p := range scan.AddedPlugins {
					if !selectedSet[p] {
						unselectedPlugins = append(unselectedPlugins, p)
					}
				}
			}
		}

		hasPluginChanges := len(selectedAdd) > 0 || len(selectedRemove) > 0
		if !hasPluginChanges && !hasNonPluginChanges {
			return pushUnpushedOrNoop(syncDir, pushForceFlag)
		}

		// Summarize non-plugin changes being included.
		if hasNonPluginChanges {
			var kinds []string
			if updatePerms {
				kinds = append(kinds, "permissions")
			}
			if updateClaudeMD {
				kinds = append(kinds, "CLAUDE.md")
			}
			if updateMCP {
				kinds = append(kinds, "MCP servers")
			}
			if updateKB {
				kinds = append(kinds, "keybindings")
			}
			if updateCmds {
				kinds = append(kinds, "commands")
			}
			if updateSkills {
				kinds = append(kinds, "skills")
			}
			fmt.Printf("Including config changes: %s\n", strings.Join(kinds, ", "))
		}

		// Profile routing prompt (only when adding plugins interactively).
		if len(selectedAdd) > 0 && !pushAll {
			profileNames, err := profiles.ListProfiles(syncDir)
			if err != nil {
				return err
			}
			if len(profileNames) > 0 {
				options := make([]huh.Option[string], 0, len(profileNames)+1)
				options = append(options, huh.NewOption("Base config (shared by all profiles)", ""))
				for _, name := range profileNames {
					options = append(options, huh.NewOption("Profile: "+name, name))
				}

				err := huh.NewForm(
					huh.NewGroup(
						huh.NewSelect[string]().
							Title("Add selected plugins to:").
							Options(options...).
							Value(&profileTarget),
					),
				).Run()
				if err != nil {
					// Esc on profile prompt defaults to base config.
					profileTarget = ""
				}
			}
		}

		headBefore, _ := git.RevParse(syncDir, "HEAD")
		err = commands.PushApply(commands.PushApplyOptions{
			ClaudeDir:         claudeDir,
			SyncDir:           syncDir,
			AddPlugins:        selectedAdd,
			RemovePlugins:     selectedRemove,
			ExcludePlugins:    unselectedPlugins,
			ProfileTarget:     profileTarget,
			Message:           pushMessage,
			UpdatePermissions: updatePerms,
			UpdateClaudeMD:    updateClaudeMD,
			UpdateMCP:         updateMCP,
			UpdateKeybindings: updateKB,
			UpdateCommands:    updateCmds,
			UpdateSkills:      updateSkills,
			Force:             pushForceFlag,
		})
		if err != nil {
			var nffErr *git.NonFastForwardError
			if errors.As(err, &nffErr) {
				if handleErr := handlePushRejection(syncDir); handleErr != nil {
					resetIfNewCommit(syncDir, headBefore)
					return handleErr
				}
				fmt.Println("Changes pushed successfully.")
				return nil
			}
			resetIfNewCommit(syncDir, headBefore)
			return err
		}

		fmt.Println("Changes pushed successfully.")
		return nil
	},
}

// pushUnpushedOrNoop pushes any existing unpushed commits, or prints a no-op
// message. Used when there are no new config changes to commit.
func pushUnpushedOrNoop(syncDir string, force bool) error {
	if git.HasRemote(syncDir, "origin") && git.HasUnpushedCommits(syncDir) {
		fmt.Println("No new changes. Pushing existing commits...")
		var pushErr error
		if !git.HasUpstream(syncDir) {
			branch, err := git.CurrentBranch(syncDir)
			if err != nil {
				return fmt.Errorf("detecting branch: %w", err)
			}
			if force {
				pushErr = git.ForcePushWithUpstream(syncDir, "origin", branch)
			} else {
				pushErr = git.PushWithUpstream(syncDir, "origin", branch)
			}
		} else {
			if force {
				pushErr = git.ForcePush(syncDir)
			} else {
				pushErr = git.Push(syncDir)
			}
		}
		if pushErr != nil {
			var nffErr *git.NonFastForwardError
			if errors.As(pushErr, &nffErr) {
				return handlePushRejection(syncDir)
			}
			return fmt.Errorf("pushing: %w", pushErr)
		}
		fmt.Println("Pushed to remote.")
		return nil
	}
	fmt.Println("Nothing to push. Everything matches config.")
	return nil
}

// resetIfNewCommit undoes the last commit if PushApply created one before
// the push failed. Changes remain staged for the next push attempt.
func resetIfNewCommit(syncDir, headBefore string) {
	headAfter, err := git.RevParse(syncDir, "HEAD")
	if err == nil && headBefore != headAfter {
		git.ResetSoftHead(syncDir)
	}
}

// handlePushRejection is called when a push is rejected because the remote has
// diverged. It shows the remote commit log and offers Rebase/Force/Abort.
func handlePushRejection(syncDir string) error {
	branch, err := git.CurrentBranch(syncDir)
	if err != nil {
		return fmt.Errorf("detecting branch: %w", err)
	}
	hasUpstream := git.HasUpstream(syncDir)

	// Show remote commit log.
	fmt.Println("\nRemote has commits not in your local repo:")
	log, logErr := git.RemoteLog(syncDir, "origin", branch, 5)
	if logErr == nil {
		for _, line := range strings.Split(log, "\n") {
			if line != "" {
				fmt.Println("  " + line)
			}
		}
	} else {
		fmt.Println("  (could not read remote log)")
	}
	fmt.Println()

	var choice string
	promptErr := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("How would you like to proceed?").
				Options(
					huh.NewOption("Rebase on top (keeps remote commits)", "rebase"),
					huh.NewOption("Force overwrite (replaces remote)", "force"),
					huh.NewOption("Abort", "abort"),
				).
				Value(&choice),
		),
	).Run()
	if promptErr != nil {
		return fmt.Errorf("push aborted")
	}

	switch choice {
	case "rebase":
		stashed := git.Stash(syncDir)
		var rebaseErr error
		if hasUpstream {
			rebaseErr = git.PullRebase(syncDir)
		} else {
			rebaseErr = git.PullRebaseFrom(syncDir, "origin", branch)
		}
		if stashed {
			git.StashPop(syncDir)
		}
		if rebaseErr != nil {
			return fmt.Errorf("rebase failed: %v", rebaseErr)
		}
		if hasUpstream {
			if err := git.Push(syncDir); err != nil {
				return fmt.Errorf("push after rebase failed: %v", err)
			}
		} else {
			if err := git.PushWithUpstream(syncDir, "origin", branch); err != nil {
				return fmt.Errorf("push after rebase failed: %v", err)
			}
		}
		fmt.Println("Rebased and pushed successfully.")
	case "force":
		if hasUpstream {
			if err := git.ForcePush(syncDir); err != nil {
				return fmt.Errorf("force push failed: %v", err)
			}
		} else {
			if err := git.ForcePushWithUpstream(syncDir, "origin", branch); err != nil {
				return fmt.Errorf("force push failed: %v", err)
			}
		}
		fmt.Println("Force pushed successfully.")
	default:
		return fmt.Errorf("push aborted")
	}
	return nil
}

// ensureRemote checks if origin is configured; if not, prompts or uses --remote flag.
func ensureRemote(syncDir string) error {
	if git.HasRemote(syncDir, "origin") {
		return nil
	}

	remoteURL := pushRemote
	if remoteURL == "" {
		err := huh.NewForm(
			huh.NewGroup(
				huh.NewInput().
					Title("No remote configured. Enter git remote URL:").
					Value(&remoteURL),
			),
		).Run()
		if err != nil {
			return err
		}
	}

	if remoteURL == "" {
		return fmt.Errorf("remote URL is required for push")
	}

	if err := git.RemoteAdd(syncDir, "origin", remoteURL); err != nil {
		return fmt.Errorf("adding remote: %w", err)
	}
	fmt.Println("✓ Remote added:", remoteURL)
	return nil
}

func init() {
	pushCmd.Flags().StringVarP(&pushMessage, "message", "m", "", "Commit message")
	pushCmd.Flags().BoolVar(&pushAll, "all", false, "Push all changes without interactive selection")
	pushCmd.Flags().StringVarP(&pushRemote, "remote", "r", "", "Git remote URL (used if no remote is configured)")
	pushCmd.Flags().BoolVar(&pushAutoFlag, "auto", false, "Auto mode: push all changes without prompts")
	pushCmd.Flags().BoolVarP(&pushQuietFlag, "quiet", "q", false, "Suppress output")
	pushCmd.Flags().BoolVarP(&pushForceFlag, "force", "f", false, "Force push (use when remote has diverged, e.g. after re-creating config)")
}
