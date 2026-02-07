package main

import (
	"fmt"

	"github.com/ruminaider/claude-sync/internal/commands"
	"github.com/ruminaider/claude-sync/internal/paths"
	"github.com/spf13/cobra"
)

var pinCmd = &cobra.Command{
	Use:   "pin <plugin> [version]",
	Short: "Pin a plugin to a specific version",
	Long:  "Pin a plugin to a specific version, preventing it from being auto-updated. If no version is specified, defaults to \"latest\".",
	Args:  cobra.RangeArgs(1, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		pluginKey := args[0]
		version := "latest"
		if len(args) > 1 {
			version = args[1]
		}

		if err := commands.Pin(paths.SyncDir(), pluginKey, version); err != nil {
			return err
		}

		fmt.Printf("Pinned %s to %s\n", pluginKey, version)
		return nil
	},
}
