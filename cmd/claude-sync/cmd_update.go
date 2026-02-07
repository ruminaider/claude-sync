package main

import (
	"fmt"
	"os"

	"github.com/ruminaider/claude-sync/internal/commands"
	"github.com/ruminaider/claude-sync/internal/paths"
	"github.com/spf13/cobra"
)

var updateQuietFlag bool

var updateCmd = &cobra.Command{
	Use:   "update",
	Short: "Check for and apply plugin updates",
	RunE: func(cmd *cobra.Command, args []string) error {
		claudeDir := paths.ClaudeDir()
		syncDir := paths.SyncDir()

		result, err := commands.UpdateCheck(claudeDir, syncDir)
		if err != nil {
			return err
		}

		if !result.HasUpdates() {
			if !updateQuietFlag {
				fmt.Println("No plugins to update.")
			}
			return nil
		}

		// Reinstall upstream plugins
		if len(result.UpstreamPlugins) > 0 {
			if !updateQuietFlag {
				fmt.Println("Updating upstream plugins:")
			}
			var keys []string
			for _, u := range result.UpstreamPlugins {
				keys = append(keys, u.Key)
			}
			installed, failed := commands.UpdateApply(claudeDir, syncDir, keys, updateQuietFlag)

			if !updateQuietFlag {
				if len(installed) > 0 {
					fmt.Printf("\nUpdated %d upstream plugin(s)\n", len(installed))
				}
				if len(failed) > 0 {
					fmt.Fprintf(os.Stderr, "\nFailed to update %d upstream plugin(s)\n", len(failed))
				}
			}
		}

		// Report pinned plugins (not auto-updated)
		if len(result.PinnedPlugins) > 0 && !updateQuietFlag {
			fmt.Println("\nPinned plugins (not auto-updated):")
			for _, p := range result.PinnedPlugins {
				fmt.Printf("  %s (pinned at %s, installed: %s)\n", p.Key, p.PinnedVersion, p.InstalledVersion)
			}
		}

		// Reinstall forked plugins
		if len(result.ForkedPlugins) > 0 {
			if !updateQuietFlag {
				fmt.Println("\nUpdating forked plugins:")
			}
			var forkNames []string
			for _, f := range result.ForkedPlugins {
				forkNames = append(forkNames, f.Name)
			}
			installed, failed := commands.UpdateForkedPlugins(claudeDir, syncDir, forkNames, updateQuietFlag)

			if !updateQuietFlag {
				if len(installed) > 0 {
					fmt.Printf("\nUpdated %d forked plugin(s)\n", len(installed))
				}
				if len(failed) > 0 {
					fmt.Fprintf(os.Stderr, "\nFailed to update %d forked plugin(s)\n", len(failed))
				}
			}
		}

		return nil
	},
}

func init() {
	updateCmd.Flags().BoolVarP(&updateQuietFlag, "quiet", "q", false, "Suppress output")
}
