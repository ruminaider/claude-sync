package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var version = "0.2.4"

var rootCmd = &cobra.Command{
	Use:   "claude-sync",
	Short: "Sync Claude Code configuration across machines",
	Long:  "claude-sync synchronizes Claude Code plugins, settings, and hooks across multiple machines using a git-backed config repo.",
	RunE: func(cmd *cobra.Command, args []string) error {
		// Default behavior: show status
		return statusCmd.RunE(cmd, args)
	},
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Show version",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("claude-sync %s\n", version)
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(initCmd)
	rootCmd.AddCommand(joinCmd)
	rootCmd.AddCommand(statusCmd)
	rootCmd.AddCommand(pullCmd)
	rootCmd.AddCommand(pushCmd)
	rootCmd.AddCommand(setupCmd)
	rootCmd.AddCommand(migrateCmd)
	rootCmd.AddCommand(pinCmd)
	rootCmd.AddCommand(unpinCmd)
	rootCmd.AddCommand(updateCmd)
	rootCmd.AddCommand(forkCmd)
	rootCmd.AddCommand(unforkCmd)
	rootCmd.AddCommand(profileCmd)
	rootCmd.AddCommand(approveCmd)
	rootCmd.AddCommand(rejectCmd)
	rootCmd.AddCommand(autoCommitCmd)
	rootCmd.AddCommand(mcpCmd)
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
