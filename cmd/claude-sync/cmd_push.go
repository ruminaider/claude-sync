package main

import (
	"fmt"

	"github.com/charmbracelet/huh"
	"github.com/ruminaider/claude-sync/internal/commands"
	"github.com/ruminaider/claude-sync/internal/git"
	"github.com/ruminaider/claude-sync/internal/paths"
	"github.com/spf13/cobra"
)

var pushMessage string
var pushAll bool
var pushRemote string

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

		fmt.Println("Scanning local state...")
		scan, err := commands.PushScan(claudeDir, syncDir)
		if err != nil {
			return err
		}

		if !scan.HasChanges() {
			fmt.Println("Nothing to push. Everything matches config.")
			return nil
		}

		var selectedAdd []string
		var selectedRemove []string

		if pushAll {
			selectedAdd = scan.AddedPlugins
			selectedRemove = scan.RemovedPlugins
		} else {
			if len(scan.AddedPlugins) > 0 {
				var options []huh.Option[string]
				for _, p := range scan.AddedPlugins {
					options = append(options, huh.NewOption(p, p).Selected(true))
				}
				err := huh.NewForm(
					huh.NewGroup(
						huh.NewMultiSelect[string]().
							Title("Plugins to add to config:").
							Description("Space to toggle, Enter to confirm").
							Options(options...).
							Value(&selectedAdd),
					),
				).Run()
				if err != nil {
					return err
				}
			}
		}

		if len(selectedAdd) == 0 && len(selectedRemove) == 0 {
			fmt.Println("No changes selected.")
			return nil
		}

		if err := commands.PushApply(claudeDir, syncDir, selectedAdd, selectedRemove, pushMessage); err != nil {
			return err
		}

		fmt.Println("Changes pushed successfully.")
		return nil
	},
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
}
