package main

import (
	"fmt"

	"github.com/ruminaider/claude-sync/internal/commands"
	"github.com/ruminaider/claude-sync/internal/paths"
	"github.com/spf13/cobra"
)

var rejectCmd = &cobra.Command{
	Use:   "reject",
	Short: "Reject and discard pending high-risk changes",
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := commands.Reject(paths.SyncDir()); err != nil {
			return err
		}
		fmt.Println("Pending changes rejected and discarded.")
		return nil
	},
}
