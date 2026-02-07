package main

import (
	"fmt"

	"github.com/ruminaider/claude-sync/internal/commands"
	"github.com/ruminaider/claude-sync/internal/paths"
	"github.com/spf13/cobra"
)

var unpinCmd = &cobra.Command{
	Use:   "unpin <plugin>",
	Short: "Unpin a plugin, returning it to upstream tracking",
	Long:  "Unpin a previously pinned plugin, moving it back to the upstream list so it tracks the latest version.",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		pluginKey := args[0]

		if err := commands.Unpin(paths.SyncDir(), pluginKey); err != nil {
			return err
		}

		fmt.Printf("Unpinned %s\n", pluginKey)
		return nil
	},
}
