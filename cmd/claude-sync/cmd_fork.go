package main

import (
	"fmt"

	"github.com/ruminaider/claude-sync/internal/commands"
	"github.com/ruminaider/claude-sync/internal/paths"
	"github.com/spf13/cobra"
)

var forkCmd = &cobra.Command{
	Use:   "fork <plugin-key>",
	Short: "Fork a plugin for local customization",
	Long:  "Copies a plugin from the Claude Code cache into ~/.claude-sync/plugins/ for local editing. The plugin is moved from upstream/pinned to the forked category.",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		pluginKey := args[0]
		if err := commands.Fork(paths.ClaudeDir(), paths.SyncDir(), pluginKey); err != nil {
			return err
		}
		fmt.Printf("Forked %s for customization\n", pluginKey)
		fmt.Println("Edit files in ~/.claude-sync/plugins/ and push when ready.")
		return nil
	},
}
