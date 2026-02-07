package main

import (
	"fmt"

	"github.com/ruminaider/claude-sync/internal/commands"
	"github.com/ruminaider/claude-sync/internal/paths"
	"github.com/spf13/cobra"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show sync status",
	RunE: func(cmd *cobra.Command, args []string) error {
		result, err := commands.Status(paths.ClaudeDir(), paths.SyncDir())
		if err != nil {
			return err
		}

		if len(result.Synced) > 0 {
			fmt.Println("SYNCED")
			for _, p := range result.Synced {
				fmt.Printf("  ✓ %s\n", p)
			}
			fmt.Println()
		}

		if len(result.NotInstalled) > 0 {
			fmt.Println("NOT INSTALLED (run 'claude-sync pull' to install)")
			for _, p := range result.NotInstalled {
				fmt.Printf("  ⚠️  %s\n", p)
			}
			fmt.Println()
		}

		if len(result.Untracked) > 0 {
			fmt.Println("UNTRACKED (run 'claude-sync push' to add to config)")
			for _, p := range result.Untracked {
				fmt.Printf("  ? %s\n", p)
			}
			fmt.Println()
		}

		if len(result.NotInstalled) == 0 && len(result.Untracked) == 0 {
			fmt.Println("Everything is in sync.")
		}

		return nil
	},
}
