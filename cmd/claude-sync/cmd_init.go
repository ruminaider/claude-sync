package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Create new config from current Claude Code setup",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("claude-sync init: not yet implemented")
		return nil
	},
}
