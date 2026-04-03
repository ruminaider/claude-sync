package main

import (
	"fmt"
	"os"

	"github.com/ruminaider/claude-sync/internal/commands"
	"github.com/spf13/cobra"
)

var updateForceFlag bool

var updateCmd = &cobra.Command{
	Use:   "update",
	Short: "Update claude-sync to the latest version",
	Long:  "Download and install the latest claude-sync binary from GitHub releases.",
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := commands.SelfUpdate(version, updateForceFlag); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		return nil
	},
}

func init() {
	updateCmd.Flags().BoolVar(&updateForceFlag, "force", false, "Install even if already up to date")
}
