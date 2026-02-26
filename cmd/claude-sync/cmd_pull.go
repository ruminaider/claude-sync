package main

import (
	"fmt"
	"os"

	"github.com/ruminaider/claude-sync/internal/commands"
	"github.com/ruminaider/claude-sync/internal/paths"
	"github.com/spf13/cobra"
)

var quietFlag bool
var autoFlag bool

var pullCmd = &cobra.Command{
	Use:   "pull",
	Short: "Pull latest config and apply locally",
	RunE: func(cmd *cobra.Command, args []string) error {
		var result *commands.PullResult
		var err error

		if autoFlag {
			projectDir := readCWDFromStdin()
			result, err = commands.PullWithOptions(commands.PullOptions{
				ClaudeDir:  paths.ClaudeDir(),
				SyncDir:    paths.SyncDir(),
				Quiet:      true,
				Auto:       true,
				ProjectDir: projectDir,
			})
		} else {
			if !quietFlag {
				fmt.Println("Pulling latest config...")
			}
			result, err = commands.Pull(paths.ClaudeDir(), paths.SyncDir(), quietFlag)
		}
		if err != nil {
			return err
		}

		if quietFlag || autoFlag {
			// In auto mode, still show pending high-risk warnings.
			if autoFlag && len(result.PendingHighRisk) > 0 {
				fmt.Fprintf(os.Stderr, "%d high-risk change(s) deferred. Run 'claude-sync approve' to apply:\n", len(result.PendingHighRisk))
				for _, c := range result.PendingHighRisk {
					fmt.Fprintf(os.Stderr, "  - %s\n", c.Description)
				}
			}
			return nil
		}

		printPullResult(result)
		return nil
	},
}

func init() {
	pullCmd.Flags().BoolVarP(&quietFlag, "quiet", "q", false, "Suppress output")
	pullCmd.Flags().BoolVar(&autoFlag, "auto", false, "Auto mode: apply safe changes, defer high-risk to pending")
}
