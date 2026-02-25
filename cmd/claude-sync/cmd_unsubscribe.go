package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/ruminaider/claude-sync/internal/config"
	gitpkg "github.com/ruminaider/claude-sync/internal/git"
	"github.com/ruminaider/claude-sync/internal/paths"
	"github.com/ruminaider/claude-sync/internal/subscriptions"
	"github.com/spf13/cobra"
)

var unsubscribeCmd = &cobra.Command{
	Use:   "unsubscribe <name>",
	Short: "Remove a subscription",
	Long: `Remove a subscription from your config.
Deletes the local clone and removes the entry from config.yaml.
Subscription-provided items will be removed on the next pull.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		syncDir := paths.SyncDir()
		name := args[0]

		cfgData, err := os.ReadFile(filepath.Join(syncDir, "config.yaml"))
		if err != nil {
			return fmt.Errorf("reading config: %w", err)
		}
		cfg, err := config.Parse(cfgData)
		if err != nil {
			return fmt.Errorf("parsing config: %w", err)
		}

		if _, exists := cfg.Subscriptions[name]; !exists {
			return fmt.Errorf("subscription %q not found", name)
		}

		// Remove from config.
		delete(cfg.Subscriptions, name)
		newData, err := config.Marshal(cfg)
		if err != nil {
			return fmt.Errorf("marshaling config: %w", err)
		}
		if err := os.WriteFile(filepath.Join(syncDir, "config.yaml"), newData, 0644); err != nil {
			return fmt.Errorf("writing config: %w", err)
		}

		// Remove local clone and state.
		state, _ := subscriptions.ReadState(syncDir)
		if err := subscriptions.RemoveSubscription(syncDir, name, &state); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: %v\n", err)
		}
		subscriptions.WriteState(syncDir, state)

		// Commit.
		gitpkg.Add(syncDir, "config.yaml")
		gitpkg.Add(syncDir, "subscriptions")
		gitpkg.Commit(syncDir, fmt.Sprintf("Unsubscribe from %s", name))

		fmt.Printf("Unsubscribed from %s.\n", name)
		fmt.Println("Run 'claude-sync pull' to remove subscription-provided items.")
		return nil
	},
}
