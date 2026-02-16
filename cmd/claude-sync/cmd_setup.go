package main

import (
	"fmt"

	"github.com/ruminaider/claude-sync/internal/commands"
	"github.com/ruminaider/claude-sync/internal/paths"
	"github.com/spf13/cobra"
)

var hooksFlag bool

var setupCmd = &cobra.Command{
	Use:   "setup",
	Short: "Show shell alias or register auto-sync hooks",
	RunE: func(cmd *cobra.Command, args []string) error {
		if hooksFlag {
			claudeDir := paths.ClaudeDir()
			if err := commands.SetupAutoSyncHooks(claudeDir); err != nil {
				return fmt.Errorf("registering hooks: %w", err)
			}
			fmt.Println("Auto-sync hooks registered in settings.json.")
			return nil
		}
		fmt.Print(commands.SetupShellAlias())
		return nil
	},
}

func init() {
	setupCmd.Flags().BoolVar(&hooksFlag, "hooks", false, "Register auto-sync hooks in Claude Code settings")
}
