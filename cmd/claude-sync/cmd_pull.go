package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/ruminaider/claude-sync/internal/commands"
	"github.com/ruminaider/claude-sync/internal/paths"
	"github.com/ruminaider/claude-sync/internal/plugins"
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

		claudeDir := paths.ClaudeDir()
		syncDir := paths.SyncDir()

		if autoFlag {
			projectDir := readCWDFromStdin()
			result, err = commands.PullWithOptions(commands.PullOptions{
				ClaudeDir:  claudeDir,
				SyncDir:    syncDir,
				Quiet:      true,
				Auto:       true,
				ProjectDir: projectDir,
			})
		} else {
			if !quietFlag {
				fmt.Println("Pulling latest config...")
			}
			result, err = commands.PullWithOptions(commands.PullOptions{
				ClaudeDir: claudeDir,
				SyncDir:   syncDir,
				Quiet:     quietFlag,
				DuplicateResolver: func(dupes []plugins.Duplicate) error {
					for _, d := range dupes {
						resolution, promptErr := promptDuplicateResolution(d, syncDir)
						if promptErr != nil {
							return promptErr
						}
						if applyErr := plugins.ApplyResolution(claudeDir, syncDir, resolution); applyErr != nil {
							return fmt.Errorf("resolving duplicate %s: %w", d.Name, applyErr)
						}
						if !quietFlag {
							fmt.Printf("Resolved: keeping %s\n", resolution.KeepSource)
						}
					}
					return nil
				},
				ReEvalResolver: func(signals []plugins.ReEvalSignal) error {
					for _, sig := range signals {
						action, promptErr := promptReEvaluation(sig)
						if promptErr != nil {
							return promptErr
						}
						switch action {
						case "switch":
							if switchErr := plugins.ApplyReEvalSwitch(claudeDir, syncDir, sig.PluginName); switchErr != nil {
								return switchErr
							}
							if !quietFlag {
								fmt.Printf("Switched %s to marketplace\n", sig.PluginName)
							}
						case "snooze":
							if snoozeErr := plugins.ApplySnooze(syncDir, sig.PluginName, 2); snoozeErr != nil {
								return snoozeErr
							}
							if !quietFlag {
								fmt.Printf("Snoozed %s for 2 days\n", sig.PluginName)
							}
						case "keep":
							if resetErr := plugins.ResetReEvalBaseline(syncDir, sig.PluginName); resetErr != nil {
								return resetErr
							}
						}
					}
					return nil
				},
			})
		}
		if err != nil {
			return err
		}

		if quietFlag || autoFlag {
			// In auto mode, surface duplicate plugins so the agent can help resolve.
			if autoFlag && len(result.DuplicatePlugins) > 0 {
				fmt.Fprintf(os.Stderr, "WARNING: %d duplicate plugin(s) detected — same plugin installed from multiple sources.\n", len(result.DuplicatePlugins))
				fmt.Fprintf(os.Stderr, "This causes redundant hook execution and slow exit times.\n")
				for _, d := range result.DuplicatePlugins {
					fmt.Fprintf(os.Stderr, "  %s: %s\n", d.Name, strings.Join(d.Sources, ", "))
				}
				fmt.Fprintf(os.Stderr, "To fix: set the unwanted source to false in enabledPlugins in ~/.claude/settings.json, then run 'claude-sync push'.\n")
			}
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
