package main

import (
	"fmt"
	"strings"

	"github.com/ruminaider/claude-sync/internal/commands"
	"github.com/ruminaider/claude-sync/internal/paths"
	"github.com/spf13/cobra"
)

var approveCmd = &cobra.Command{
	Use:   "approve",
	Short: "Approve and apply pending high-risk changes",
	RunE: func(cmd *cobra.Command, args []string) error {
		result, err := commands.Approve(paths.ClaudeDir(), paths.SyncDir())
		if err != nil {
			return err
		}

		if result.PermissionsApplied {
			fmt.Println("\u2713 Permissions applied")
		}
		if len(result.MCPApplied) > 0 {
			fmt.Printf("\u2713 MCP servers applied: %s\n", strings.Join(result.MCPApplied, ", "))
		}
		if len(result.HooksApplied) > 0 {
			fmt.Printf("\u2713 Hooks applied: %s\n", strings.Join(result.HooksApplied, ", "))
		}

		fmt.Println("\nAll pending changes approved and applied.")
		return nil
	},
}
