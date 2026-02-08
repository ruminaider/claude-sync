package main

import (
	"fmt"

	"github.com/ruminaider/claude-sync/internal/commands"
	"github.com/ruminaider/claude-sync/internal/paths"
	"github.com/spf13/cobra"
)

var initRemote string

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Create new config from current Claude Code setup",
	RunE: func(cmd *cobra.Command, args []string) error {
		result, err := commands.Init(paths.ClaudeDir(), paths.SyncDir(), initRemote)
		if err != nil {
			return err
		}
		fmt.Println("✓ Created ~/.claude-sync/")
		fmt.Println("✓ Generated config.yaml from current Claude Code setup")
		fmt.Println("✓ Initialized git repository")

		if len(result.Upstream) > 0 {
			fmt.Printf("\n  Upstream:    %d plugin(s)\n", len(result.Upstream))
		}
		if len(result.AutoForked) > 0 {
			fmt.Printf("  Auto-forked: %d plugin(s) (non-portable marketplace)\n", len(result.AutoForked))
			for _, p := range result.AutoForked {
				fmt.Printf("    → %s\n", p)
			}
		}
		if len(result.Skipped) > 0 {
			fmt.Printf("  Skipped:     %d plugin(s) (local scope)\n", len(result.Skipped))
			for _, p := range result.Skipped {
				fmt.Printf("    → %s\n", p)
			}
		}

		if result.RemotePushed {
			fmt.Println()
			fmt.Println("✓ Pushed to remote")
			fmt.Println()
			fmt.Println("Next steps:")
			fmt.Println("  On another machine: claude-sync join", initRemote)
		} else {
			fmt.Println()
			fmt.Println("Next steps:")
			fmt.Println("  1. Review config: cat ~/.claude-sync/config.yaml")
			fmt.Println("  2. Push: claude-sync push -m \"Initial config\"")
		}
		return nil
	},
}

func init() {
	initCmd.Flags().StringVarP(&initRemote, "remote", "r", "", "Git remote URL to add as origin and push to")
}
