package main

import (
	"fmt"

	"github.com/ruminaider/claude-sync/internal/commands"
	"github.com/ruminaider/claude-sync/internal/paths"
	"github.com/spf13/cobra"
)

var conflictsCmd = &cobra.Command{
	Use:   "conflicts",
	Short: "Manage pending merge conflicts",
	Long:  "List, resolve, or discard pending merge conflicts from config synchronization.",
	RunE: func(cmd *cobra.Command, args []string) error {
		syncDir := paths.SyncDir()
		conflicts, err := commands.ListPendingConflicts(syncDir)
		if err != nil {
			return err
		}
		if len(conflicts) == 0 {
			fmt.Println("No pending conflicts.")
			return nil
		}
		fmt.Printf("%d pending conflict(s):\n\n", len(conflicts))
		for i, c := range conflicts {
			fmt.Printf("  [%d] %s\n", i, c.Key)
			fmt.Printf("      Local:  %s\n", string(c.LocalValue))
			fmt.Printf("      Remote: %s\n", string(c.RemoteValue))
			fmt.Println()
		}
		return nil
	},
}

var conflictsDiscardCmd = &cobra.Command{
	Use:   "discard",
	Short: "Discard all pending conflicts (keep current config)",
	RunE: func(cmd *cobra.Command, args []string) error {
		syncDir := paths.SyncDir()
		if !commands.HasPendingConflicts(syncDir) {
			fmt.Println("No pending conflicts.")
			return nil
		}
		if err := commands.DiscardConflicts(syncDir); err != nil {
			return err
		}
		fmt.Println("All pending conflicts discarded.")
		return nil
	},
}

func init() {
	conflictsCmd.AddCommand(conflictsDiscardCmd)
}
