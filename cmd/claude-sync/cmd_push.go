package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

var pushMessage string
var pushAll bool

var pushCmd = &cobra.Command{
	Use:   "push",
	Short: "Push changes to remote",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("claude-sync push: not yet implemented")
		return nil
	},
}

func init() {
	pushCmd.Flags().StringVarP(&pushMessage, "message", "m", "", "Commit message")
	pushCmd.Flags().BoolVar(&pushAll, "all", false, "Push all changes without interactive selection")
}
