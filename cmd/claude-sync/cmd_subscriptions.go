package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/ruminaider/claude-sync/internal/config"
	"github.com/ruminaider/claude-sync/internal/paths"
	"github.com/ruminaider/claude-sync/internal/subscriptions"
	"github.com/spf13/cobra"
)

var subscriptionsCmd = &cobra.Command{
	Use:     "subscriptions",
	Aliases: []string{"subs"},
	Short:   "List active subscriptions",
	RunE: func(cmd *cobra.Command, args []string) error {
		syncDir := paths.SyncDir()

		cfgData, err := os.ReadFile(filepath.Join(syncDir, "config.yaml"))
		if err != nil {
			return fmt.Errorf("reading config: %w", err)
		}
		cfg, err := config.Parse(cfgData)
		if err != nil {
			return fmt.Errorf("parsing config: %w", err)
		}

		if len(cfg.Subscriptions) == 0 {
			fmt.Println("No subscriptions configured.")
			fmt.Println("Run 'claude-sync subscribe <url>' to add one.")
			return nil
		}

		state, _ := subscriptions.ReadState(syncDir)

		fmt.Println("SUBSCRIPTIONS")

		// Sort names for deterministic output.
		names := make([]string, 0, len(cfg.Subscriptions))
		for name := range cfg.Subscriptions {
			names = append(names, name)
		}
		sort.Strings(names)

		for _, name := range names {
			sub := cfg.Subscriptions[name]
			ref := sub.Ref
			if ref == "" {
				ref = "main"
			}

			fmt.Printf("  %s  %s (%s)\n", name, sub.URL, ref)

			// Show last-fetched info.
			if subState, ok := state.Subscriptions[name]; ok {
				ago := time.Since(subState.LastFetched).Truncate(time.Second)
				fmt.Printf("    Last fetched: %s ago (%s)\n", formatDuration(ago), shortSHA(subState.CommitSHA))
			} else {
				fmt.Println("    Last fetched: never")
			}

			// Show category summary.
			if sub.Categories != nil {
				var catParts []string
				for cat, mode := range sub.Categories {
					modeStr := fmt.Sprintf("%v", mode)
					displayName := titleCase(cat)

					if excludes, ok := sub.Exclude[cat]; ok && len(excludes) > 0 {
						catParts = append(catParts, fmt.Sprintf("%s: %s (excluding: %s)",
							displayName, modeStr, strings.Join(excludes, ", ")))
					} else if includes, ok := sub.Include[cat]; ok && len(includes) > 0 {
						catParts = append(catParts, fmt.Sprintf("%s: %d (%s)",
							displayName, len(includes), strings.Join(includes, ", ")))
					} else {
						catParts = append(catParts, fmt.Sprintf("%s: %s", displayName, modeStr))
					}
				}
				sort.Strings(catParts)
				for _, part := range catParts {
					fmt.Printf("    %s\n", part)
				}
			}

			fmt.Println()
		}

		return nil
	},
}

func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("%dh", int(d.Hours()))
	}
	return fmt.Sprintf("%dd", int(d.Hours()/24))
}

func shortSHA(sha string) string {
	if len(sha) > 7 {
		return sha[:7]
	}
	return sha
}

func titleCase(s string) string {
	if len(s) == 0 {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}
