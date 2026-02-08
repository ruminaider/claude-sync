package main

import (
	"fmt"

	"github.com/charmbracelet/huh"
	"github.com/ruminaider/claude-sync/internal/commands"
	"github.com/ruminaider/claude-sync/internal/paths"
	"github.com/spf13/cobra"
)

var joinClean bool
var joinKeepLocal bool

var joinCmd = &cobra.Command{
	Use:   "join <url>",
	Short: "Join existing config repo",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		repoURL := args[0]
		fmt.Printf("Cloning config from %s...\n", repoURL)

		result, err := commands.Join(repoURL, paths.ClaudeDir(), paths.SyncDir())
		if err != nil {
			return err
		}

		fmt.Println("✓ Cloned config repo to ~/.claude-sync/")

		if len(result.LocalOnly) > 0 && !joinKeepLocal {
			fmt.Printf("\nFound %d locally installed plugin(s) not in the remote config:\n", len(result.LocalOnly))
			for _, p := range result.LocalOnly {
				fmt.Printf("  • %s\n", p)
			}

			var toRemove []string

			if joinClean {
				toRemove = result.LocalOnly
			} else {
				toRemove, err = promptLocalPluginCleanup(result.LocalOnly)
				if err != nil {
					// User cancelled — keep them all.
					toRemove = nil
				}
			}

			if len(toRemove) > 0 {
				removed, failed := commands.JoinCleanup(toRemove)
				for _, p := range removed {
					fmt.Printf("  ✓ Removed %s\n", p)
				}
				for _, p := range failed {
					fmt.Printf("  ✗ Failed to remove %s\n", p)
				}
			}
		}

		fmt.Println()
		fmt.Println("Run 'claude-sync pull' to apply the config.")
		return nil
	},
}

// promptLocalPluginCleanup asks the user what to do with local-only plugins.
func promptLocalPluginCleanup(plugins []string) ([]string, error) {
	var choice string
	fmt.Println()
	err := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("What would you like to do with these plugins?").
				Options(
					huh.NewOption("Remove all", "all"),
					huh.NewOption("Choose which to remove", "some"),
					huh.NewOption("Keep all", "keep"),
				).
				Value(&choice),
		),
	).Run()
	if err != nil {
		return nil, err
	}

	switch choice {
	case "all":
		return plugins, nil
	case "keep":
		return nil, nil
	case "some":
		var selected []string
		var options []huh.Option[string]
		for _, p := range plugins {
			options = append(options, huh.NewOption(p, p))
		}
		err := huh.NewForm(
			huh.NewGroup(
				huh.NewMultiSelect[string]().
					Title("Select plugins to remove:").
					Description("Space to toggle, Enter to confirm").
					Options(options...).
					Value(&selected),
			),
		).Run()
		if err != nil {
			return nil, err
		}
		return selected, nil
	}

	return nil, nil
}

func init() {
	joinCmd.Flags().BoolVar(&joinClean, "clean", false, "Automatically remove local-only plugins not in the remote config")
	joinCmd.Flags().BoolVar(&joinKeepLocal, "keep-local", false, "Keep all locally installed plugins without prompting")
}
