package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ruminaider/claude-sync/cmd/claude-sync/tui"
	"github.com/ruminaider/claude-sync/internal/config"
	gitpkg "github.com/ruminaider/claude-sync/internal/git"
	"github.com/ruminaider/claude-sync/internal/paths"
	"github.com/ruminaider/claude-sync/internal/subscriptions"
	"github.com/spf13/cobra"
)

var subscribeCmd = &cobra.Command{
	Use:   "subscribe <url> [name]",
	Short: "Subscribe to another team's claude-sync config",
	Long: `Subscribe to another team's claude-sync config repo.
Opens a TUI to browse and select which items (MCP servers, plugins, settings, etc.) to include.
The optional name argument sets the subscription name; defaults to the repo name.`,
	Args: cobra.RangeArgs(1, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		syncDir := paths.SyncDir()
		if _, err := os.Stat(syncDir); os.IsNotExist(err) {
			return fmt.Errorf("claude-sync not initialized. Run 'claude-sync init' first")
		}

		url := args[0]
		name := deriveSubName(url)
		if len(args) > 1 {
			name = args[1]
		}

		// Check for duplicate.
		cfgData, err := os.ReadFile(filepath.Join(syncDir, "config.yaml"))
		if err != nil {
			return fmt.Errorf("reading config: %w", err)
		}
		cfg, err := config.Parse(cfgData)
		if err != nil {
			return fmt.Errorf("parsing config: %w", err)
		}
		if _, exists := cfg.Subscriptions[name]; exists {
			return fmt.Errorf("subscription %q already exists. Use 'claude-sync unsubscribe %s' first", name, name)
		}

		// Shallow clone to temp dir, then read config.
		fmt.Printf("Fetching %s...\n", url)
		subDir := subscriptions.SubDir(syncDir, name)
		if err := gitpkg.ShallowClone(url, subDir, "main"); err != nil {
			return fmt.Errorf("cloning: %w", err)
		}

		remoteCfgData, err := os.ReadFile(filepath.Join(subDir, "config.yaml"))
		if err != nil {
			os.RemoveAll(subDir)
			return fmt.Errorf("subscription repo has no config.yaml: %w", err)
		}
		remoteCfg, err := config.Parse(remoteCfgData)
		if err != nil {
			os.RemoveAll(subDir)
			return fmt.Errorf("parsing remote config: %w", err)
		}

		// Open TUI for selection.
		model := tui.NewSubscribeModel(name, url, remoteCfg)
		p := tea.NewProgram(model, tea.WithAltScreen())
		finalModel, err := p.Run()
		if err != nil {
			return fmt.Errorf("TUI error: %w", err)
		}

		result := finalModel.(tui.SubscribeModel).Result()
		if result == nil {
			fmt.Println("Cancelled.")
			os.RemoveAll(subDir)
			return nil
		}

		// Build the subscription entry.
		entry := config.SubscriptionEntry{
			URL: url,
		}
		if len(result.Categories) > 0 {
			entry.Categories = make(map[string]any, len(result.Categories))
			for k, v := range result.Categories {
				entry.Categories[k] = v
			}
		}
		if len(result.Exclude) > 0 {
			entry.Exclude = result.Exclude
		}
		if len(result.Include) > 0 {
			entry.Include = result.Include
		}

		// Write subscription to config.yaml.
		if cfg.Subscriptions == nil {
			cfg.Subscriptions = make(map[string]config.SubscriptionEntry)
		}
		cfg.Subscriptions[name] = entry

		newData, err := config.Marshal(cfg)
		if err != nil {
			return fmt.Errorf("marshaling config: %w", err)
		}
		if err := os.WriteFile(filepath.Join(syncDir, "config.yaml"), newData, 0644); err != nil {
			return fmt.Errorf("writing config: %w", err)
		}

		// Initialize subscription state.
		state, _ := subscriptions.ReadState(syncDir)
		sha, _ := gitpkg.HeadSHA(subDir)

		// Build initial accepted items from selections.
		acceptedItems := make(map[string][]string)
		for cat, items := range result.Include {
			acceptedItems[cat] = items
		}
		// For "all" categories, all items are accepted.
		for cat, mode := range result.Categories {
			if mode == "all" {
				acceptedItems[cat] = tui.CategoryItems(remoteCfg, cat)
			}
		}
		subscriptions.UpdateState(&state, name, sha, acceptedItems)
		subscriptions.WriteState(syncDir, state)

		// Commit.
		gitpkg.Add(syncDir, "config.yaml")
		gitpkg.Add(syncDir, "subscriptions")
		gitpkg.Commit(syncDir, fmt.Sprintf("Subscribe to %s", name))

		fmt.Printf("Subscribed to %s.\n", name)
		fmt.Println("Run 'claude-sync pull' to apply subscription items.")
		return nil
	},
}

// deriveSubName extracts a short name from a git URL.
func deriveSubName(url string) string {
	// Handle SSH URLs: git@github.com:org/repo.git
	if idx := strings.LastIndex(url, "/"); idx >= 0 {
		name := url[idx+1:]
		name = strings.TrimSuffix(name, ".git")
		return name
	}
	if idx := strings.LastIndex(url, ":"); idx >= 0 {
		name := url[idx+1:]
		name = strings.TrimSuffix(name, ".git")
		if slash := strings.LastIndex(name, "/"); slash >= 0 {
			name = name[slash+1:]
		}
		return name
	}
	return "subscription"
}
