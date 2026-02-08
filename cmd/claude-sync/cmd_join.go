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

			shouldClean := joinClean
			if !joinClean {
				fmt.Println()
				err := huh.NewForm(
					huh.NewGroup(
						huh.NewConfirm().
							Title("Remove these local-only plugins?").
							Description("They are not part of the synced config and may cause conflicts.").
							Affirmative("Yes, remove them").
							Negative("No, keep them").
							Value(&shouldClean),
					),
				).Run()
				if err != nil {
					// User cancelled — keep them.
					shouldClean = false
				}
			}

			if shouldClean {
				removed, failed := commands.JoinCleanup(result.LocalOnly)
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

func init() {
	joinCmd.Flags().BoolVar(&joinClean, "clean", false, "Automatically remove local-only plugins not in the remote config")
	joinCmd.Flags().BoolVar(&joinKeepLocal, "keep-local", false, "Keep all locally installed plugins without prompting")
}
