package main

import (
	"fmt"

	"github.com/ruminaider/claude-sync/internal/commands"
	"github.com/ruminaider/claude-sync/internal/paths"
	"github.com/spf13/cobra"
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Create new config from current Claude Code setup",
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := commands.Init(paths.ClaudeDir(), paths.SyncDir()); err != nil {
			return err
		}
		fmt.Println("✓ Created ~/.claude-sync/")
		fmt.Println("✓ Generated config.yaml from current Claude Code setup")
		fmt.Println("✓ Initialized git repository")
		fmt.Println()
		fmt.Println("Next steps:")
		fmt.Println("  1. Review config: cat ~/.claude-sync/config.yaml")
		fmt.Println("  2. Add remote: cd ~/.claude-sync && git remote add origin <url>")
		fmt.Println("  3. Push: claude-sync push -m \"Initial config\"")
		return nil
	},
}
