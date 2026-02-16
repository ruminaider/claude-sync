package main

import (
	"fmt"

	"github.com/ruminaider/claude-sync/internal/commands"
	"github.com/ruminaider/claude-sync/internal/paths"
	"github.com/spf13/cobra"
)

var autoCommitIfChanged bool

var autoCommitCmd = &cobra.Command{
	Use:   "auto-commit",
	Short: "Auto-commit local changes to sync repo",
	RunE: func(cmd *cobra.Command, args []string) error {
		result, err := commands.AutoCommit(paths.ClaudeDir(), paths.SyncDir())
		if err != nil {
			return err
		}

		if !result.Changed {
			if !autoCommitIfChanged {
				fmt.Println("No changes detected.")
			}
			return nil
		}

		fmt.Printf("Committed: %s\n", result.CommitMessage)
		return nil
	},
}

func init() {
	autoCommitCmd.Flags().BoolVar(&autoCommitIfChanged, "if-changed", false, "Only output if changes were committed")
}
