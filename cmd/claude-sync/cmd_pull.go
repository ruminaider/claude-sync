package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/ruminaider/claude-sync/internal/commands"
	"github.com/ruminaider/claude-sync/internal/paths"
	"github.com/spf13/cobra"
)

var quietFlag bool

var pullCmd = &cobra.Command{
	Use:   "pull",
	Short: "Pull latest config and apply locally",
	RunE: func(cmd *cobra.Command, args []string) error {
		if !quietFlag {
			fmt.Println("Pulling latest config...")
		}

		result, err := commands.Pull(paths.ClaudeDir(), paths.SyncDir(), quietFlag)
		if err != nil {
			return err
		}

		if quietFlag {
			return nil
		}

		if len(result.Installed) > 0 {
			fmt.Printf("\n✓ %d plugin(s) installed\n", len(result.Installed))
		}

		if len(result.SettingsApplied) > 0 {
			fmt.Printf("✓ Settings applied: %s\n", strings.Join(result.SettingsApplied, ", "))
		}
		if len(result.HooksApplied) > 0 {
			fmt.Printf("✓ Hooks applied: %s\n", strings.Join(result.HooksApplied, ", "))
		}

		if len(result.Failed) > 0 {
			fmt.Fprintf(os.Stderr, "\n⚠️  %d plugin(s) failed:\n", len(result.Failed))
			for _, p := range result.Failed {
				fmt.Fprintf(os.Stderr, "  • %s\n", p)
			}
		}

		nothingChanged := len(result.ToInstall) == 0 && len(result.SettingsApplied) == 0 && len(result.HooksApplied) == 0
		if len(result.Failed) > 0 {
			fmt.Fprintf(os.Stderr, "\nSome plugins could not be installed. Check the errors above.\n")
		} else if nothingChanged {
			fmt.Println("Everything up to date.")
		}

		if len(result.Untracked) > 0 && len(result.Failed) == 0 {
			fmt.Printf("\nNote: %d plugin(s) installed locally but not in config:\n", len(result.Untracked))
			for _, p := range result.Untracked {
				fmt.Printf("  • %s\n", p)
			}
			fmt.Println("Run 'claude-sync push' to add them, or keep as local-only.")
		}

		return nil
	},
}

func init() {
	pullCmd.Flags().BoolVarP(&quietFlag, "quiet", "q", false, "Suppress output")
}
