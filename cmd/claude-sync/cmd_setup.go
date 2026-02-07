package main

import (
	"fmt"

	"github.com/ruminaider/claude-sync/internal/commands"
	"github.com/spf13/cobra"
)

var setupCmd = &cobra.Command{
	Use:   "setup",
	Short: "Show shell alias setup instructions",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Print(commands.SetupShellAlias())
	},
}
