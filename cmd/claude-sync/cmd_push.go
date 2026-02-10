package main

import (
	"fmt"

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

		if len(selectedAdd) == 0 && len(selectedRemove) == 0 {
			fmt.Println("No changes selected.")
			return nil
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

		if err := commands.PushApply(commands.PushApplyOptions{
			ClaudeDir:      claudeDir,
			SyncDir:        syncDir,
			AddPlugins:     selectedAdd,
			RemovePlugins:  selectedRemove,
			ExcludePlugins: unselectedPlugins,
			ProfileTarget:  profileTarget,
			Message:        pushMessage,
		}); err != nil {
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
