package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

var quietFlag bool

var pullCmd = &cobra.Command{
	Use:   "pull",
	Short: "Pull latest config and apply locally",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("claude-sync pull: not yet implemented")
		return nil
	},
}

func init() {
	pullCmd.Flags().BoolVarP(&quietFlag, "quiet", "q", false, "Suppress output")
}
