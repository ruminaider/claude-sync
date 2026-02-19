package main

import "github.com/spf13/cobra"

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Manage global claude-sync configuration",
	Long:  "Commands for creating and joining claude-sync configuration repositories.",
}
