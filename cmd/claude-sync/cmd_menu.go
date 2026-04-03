package main

import (
	"fmt"
	"os"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/term"
	"github.com/ruminaider/claude-sync/cmd/claude-sync/tui"
	"github.com/ruminaider/claude-sync/internal/commands"
	"github.com/ruminaider/claude-sync/internal/config"
	"github.com/ruminaider/claude-sync/internal/paths"
	"github.com/ruminaider/claude-sync/internal/profiles"
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

		vc := commands.CheckForUpdate(paths.SyncDir(), version)
		model.SetUpdateInfo(vc.UpdateAvailable, vc.LatestVersion)

		p := tea.NewProgram(model, tea.WithAltScreen())
		finalModel, err := p.Run()
		if err != nil {
			return err
		}

		app := finalModel.(tui.AppModel)

		// Check if we need to launch the config editor
		if app.LaunchConfigEditor {
			// Load existing config and profiles so the editor shows
			// current state instead of behaving like a fresh create.
			var existingConfig *config.Config
			var existingProfiles map[string]profiles.Profile

			syncDir := paths.SyncDir()
			if data, err := os.ReadFile(filepath.Join(syncDir, "config.yaml")); err == nil {
				if cfg, err := config.Parse(data); err == nil {
					existingConfig = &cfg
				}
			}
			if names, err := profiles.ListProfiles(syncDir); err == nil {
				for _, name := range names {
					if p, err := profiles.ReadProfile(syncDir, name); err == nil {
						if existingProfiles == nil {
							existingProfiles = make(map[string]profiles.Profile)
						}
						existingProfiles[name] = p
					}
				}
			}

			if err := runConfigFlow(true, existingConfig, existingProfiles); err != nil {
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
