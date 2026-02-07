package main

import (
	"fmt"

	"github.com/ruminaider/claude-sync/internal/commands"
	"github.com/ruminaider/claude-sync/internal/paths"
	"github.com/spf13/cobra"
)

var joinCmd = &cobra.Command{
	Use:   "join <url>",
	Short: "Join existing config repo",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		repoURL := args[0]
		fmt.Printf("Cloning config from %s...\n", repoURL)

		if err := commands.Join(repoURL, paths.ClaudeDir(), paths.SyncDir()); err != nil {
			return err
		}

		fmt.Println("✓ Cloned config repo to ~/.claude-sync/")
		fmt.Println()
		fmt.Println("Run 'claude-sync pull' to apply the config.")
		return nil
	},
}
