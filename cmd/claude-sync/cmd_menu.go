package main

import (
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

		// Launch AppModel
		model := tui.NewAppModel(state)
		model.SetVersion(version)
		model.SetClaudeDir(paths.ClaudeDir())
		model.SetSyncDir(paths.SyncDir())

		p := tea.NewProgram(model, tea.WithAltScreen())
		finalModel, err := p.Run()
		if err != nil {
			return err
		}

		app := finalModel.(tui.AppModel)

		// Check if we need to launch the config editor
		if app.LaunchConfigEditor {
			// Run config editor (same as "config update" flow)
			if err := runConfigFlow(true, nil, nil); err != nil {
				fmt.Fprintf(os.Stderr, "\n  \u2717 %v\n", err)
				if help := tui.ErrorGuidance(tui.ActionConfigUpdate, err); help != nil {
					fmt.Fprintf(os.Stderr, "    %s\n", help.Why)
					fmt.Fprintf(os.Stderr, "    \u2192 %s\n", help.Action)
				}
				fmt.Fprintf(os.Stderr, "\n")
			}
			continue // re-launch AppModel with refreshed state
		}

		// Normal quit
		return nil
	}
}
