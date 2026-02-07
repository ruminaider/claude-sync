package main

import (
	"fmt"
	"strings"

	"github.com/ruminaider/claude-sync/internal/commands"
	"github.com/ruminaider/claude-sync/internal/paths"
	"github.com/spf13/cobra"
)

var unforkMarketplace string

var unforkCmd = &cobra.Command{
	Use:   "unfork <plugin-name>",
	Short: "Return a forked plugin to upstream tracking",
	Long:  "Removes the local fork from ~/.claude-sync/plugins/ and restores the plugin to the upstream category.",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		pluginName := args[0]
		marketplace := unforkMarketplace

		// If no --marketplace flag, try to parse name@marketplace from arg.
		if marketplace == "" {
			if parts := strings.SplitN(pluginName, "@", 2); len(parts) == 2 {
				pluginName = parts[0]
				marketplace = parts[1]
			}
		}

		if marketplace == "" {
			return fmt.Errorf("marketplace is required: use --marketplace flag or provide name@marketplace")
		}

		if err := commands.Unfork(paths.SyncDir(), pluginName, marketplace); err != nil {
			return err
		}
		fmt.Printf("Unforked %s — returning to upstream (%s)\n", pluginName, marketplace)
		return nil
	},
}

func init() {
	unforkCmd.Flags().StringVar(&unforkMarketplace, "marketplace", "", "Marketplace to restore the plugin to")
}
