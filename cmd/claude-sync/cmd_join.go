package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

var joinCmd = &cobra.Command{
	Use:   "join <url>",
	Short: "Join existing config repo",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("claude-sync join: not yet implemented")
		return nil
	},
}
