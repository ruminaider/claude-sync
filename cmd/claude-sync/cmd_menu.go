package main

import (
	"bufio"
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/term"
	"github.com/ruminaider/claude-sync/cmd/claude-sync/tui"
	"github.com/ruminaider/claude-sync/internal/commands"
	"github.com/ruminaider/claude-sync/internal/paths"
	"github.com/spf13/cobra"
)

func runMainMenu(cmd *cobra.Command, args []string) error {
	// TTY guard: fall back to status when stdin is not a terminal
	// (piping, CI, scripts, etc.)
	if !term.IsTerminal(os.Stdin.Fd()) {
		return statusCmd.RunE(cmd, args)
	}

	for {
		// Detect current state
		state := commands.DetectMenuState(paths.ClaudeDir(), paths.SyncDir())

		// Launch menu TUI
		model := tui.NewMenuModel(state)
		model.Version = version
		p := tea.NewProgram(model, tea.WithAltScreen())
		finalModel, err := p.Run()
		if err != nil {
			return err
		}

		menu := finalModel.(tui.MenuModel)
		if menu.Quitting {
			return nil
		}

		// Execute the selected action
		action := menu.Selected
		if action.ID == "" {
			return nil
		}

		err = dispatchAction(cmd, action)

		// For CLI actions, wait for user to press Enter before returning to menu
		if action.Type == tui.ActionCLI {
			if err != nil {
				fmt.Fprintf(os.Stderr, "\nError: %v\n", err)
			}
			fmt.Print("\nPress Enter to return to menu...")
			bufio.NewReader(os.Stdin).ReadBytes('\n')
		} else if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		}
	}
}

func dispatchAction(cmd *cobra.Command, action tui.MenuAction) error {
	switch action.ID {
	// Sync
	case tui.ActionPull:
		return pullCmd.RunE(pullCmd, nil)
	case tui.ActionPush:
		return pushCmd.RunE(pushCmd, nil)
	case tui.ActionStatus:
		return statusCmd.RunE(statusCmd, nil)

	// Config
	case tui.ActionConfigCreate:
		return configCreateCmd.RunE(configCreateCmd, nil)
	case tui.ActionConfigUpdate:
		return configUpdateCmd.RunE(configUpdateCmd, nil)
	case tui.ActionConfigJoin:
		return configJoinCmd.RunE(configJoinCmd, nil)
	case tui.ActionSetup:
		return setupCmd.RunE(setupCmd, nil)

	// Plugins
	case tui.ActionSubscribe:
		return subscribeCmd.RunE(subscribeCmd, nil)
	case tui.ActionSubscriptions:
		return subscriptionsCmd.RunE(subscriptionsCmd, nil)

	// Profiles
	case tui.ActionProfileShow:
		return profileShowCmd.RunE(profileShowCmd, nil)

	// Advanced
	case tui.ActionApprove:
		return approveCmd.RunE(approveCmd, nil)
	case tui.ActionReject:
		return rejectCmd.RunE(rejectCmd, nil)
	case tui.ActionMCPImport:
		return mcpImportCmd.RunE(mcpImportCmd, nil)
	case tui.ActionConflicts:
		return conflictsCmd.RunE(conflictsCmd, nil)

	// Phase 2: Plugin management
	case tui.ActionPluginPin:
		if len(action.Args) == 0 {
			return fmt.Errorf("plugin-pin requires a plugin key")
		}
		fmt.Print("Pin version (default: latest): ")
		var version string
		fmt.Scanln(&version)
		if version == "" {
			version = "latest"
		}
		return pinCmd.RunE(pinCmd, []string{action.Args[0], version})

	case tui.ActionPluginUnpin:
		if len(action.Args) == 0 {
			return fmt.Errorf("plugin-unpin requires a plugin key")
		}
		return unpinCmd.RunE(unpinCmd, []string{action.Args[0]})

	case tui.ActionPluginFork:
		if len(action.Args) == 0 {
			return fmt.Errorf("plugin-fork requires a plugin key")
		}
		return forkCmd.RunE(forkCmd, []string{action.Args[0]})

	case tui.ActionPluginUnfork:
		if len(action.Args) == 0 {
			return fmt.Errorf("plugin-unfork requires a plugin key")
		}
		return unforkCmd.RunE(unforkCmd, []string{action.Args[0]})

	case tui.ActionPluginUpdate:
		if len(action.Args) == 0 {
			return fmt.Errorf("plugin-update requires a plugin key")
		}
		return updateCmd.RunE(updateCmd, []string{action.Args[0]})

	// Phase 2: Profile activation
	case tui.ActionProfileSet:
		if len(action.Args) == 0 {
			return fmt.Errorf("profile-set requires a profile name argument")
		}
		name := action.Args[0]
		if name == "" {
			profileSetNone = true
			defer func() { profileSetNone = false }()
			return profileSetCmd.RunE(profileSetCmd, nil)
		}
		return profileSetCmd.RunE(profileSetCmd, []string{name})

	// Phase 2: Project management
	case tui.ActionProjectInit:
		return projectInitCmd.RunE(projectInitCmd, nil)

	case tui.ActionProjectRemove:
		if len(action.Args) == 0 {
			return fmt.Errorf("project-remove requires a project path")
		}
		return projectRemoveCmd.RunE(projectRemoveCmd, []string{action.Args[0]})

	default:
		return fmt.Errorf("unknown action: %s", action.ID)
	}
}
