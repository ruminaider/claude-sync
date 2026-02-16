package main

import (
	"fmt"
	"sort"
	"strings"

	"github.com/ruminaider/claude-sync/internal/commands"
	"github.com/ruminaider/claude-sync/internal/paths"
	"github.com/spf13/cobra"
)

var jsonOutput bool

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show sync status",
	RunE: func(cmd *cobra.Command, args []string) error {
		result, err := commands.Status(paths.ClaudeDir(), paths.SyncDir())
		if err != nil {
			return err
		}

		if jsonOutput {
			data, err := result.JSON()
			if err != nil {
				return fmt.Errorf("marshaling JSON: %w", err)
			}
			fmt.Println(string(data))
			return nil
		}

		// V2 categorized display
		if strings.HasPrefix(result.ConfigVersion, "2.") {
			if err := displayV2Status(result); err != nil {
				return err
			}
		} else {
			// V1 fallback display
			if err := displayV1Status(result); err != nil {
				return err
			}
		}

		// Display pending changes if any.
		if result.PendingChanges != nil {
			displayPendingChanges(result)
		}

		return nil
	},
}

func init() {
	statusCmd.Flags().BoolVar(&jsonOutput, "json", false, "Output status as JSON")
}

func displayV1Status(result *commands.StatusResult) error {
	if len(result.Synced) > 0 {
		fmt.Println("SYNCED")
		for _, p := range result.Synced {
			fmt.Printf("  %s %s\n", checkMark, p)
		}
		fmt.Println()
	}

	if len(result.NotInstalled) > 0 {
		fmt.Println("NOT INSTALLED (run 'claude-sync pull' to install)")
		for _, p := range result.NotInstalled {
			fmt.Printf("  %s  %s\n", warningSign, p)
		}
		fmt.Println()
	}

	if len(result.Untracked) > 0 {
		fmt.Println("UNTRACKED (run 'claude-sync push' to add to config)")
		for _, p := range result.Untracked {
			fmt.Printf("  ? %s\n", p)
		}
		fmt.Println()
	}

	if len(result.NotInstalled) == 0 && len(result.Untracked) == 0 {
		fmt.Println("Everything is in sync.")
	}

	return nil
}

// Unicode symbols used in status display.
const (
	checkMark   = "\u2713" // check mark
	warningSign = "\u26A0" // warning sign
	crossMark   = "\u2717" // cross mark
)

func displayV2Status(result *commands.StatusResult) error {
	hasMissing := false

	// Upstream section
	if len(result.UpstreamSynced) > 0 || len(result.UpstreamMissing) > 0 {
		fmt.Println("UPSTREAM")
		for _, p := range result.UpstreamSynced {
			fmt.Printf("  %s %s", checkMark, p.Key)
			if p.InstalledVersion != "" {
				fmt.Printf(" (v%s)", p.InstalledVersion)
			}
			fmt.Println()
		}
		for _, p := range result.UpstreamMissing {
			fmt.Printf("  %s %s (not installed)\n", crossMark, p.Key)
			hasMissing = true
		}
		fmt.Println()
	}

	// Pinned section
	if len(result.PinnedSynced) > 0 || len(result.PinnedMissing) > 0 {
		fmt.Println("PINNED")
		for _, p := range result.PinnedSynced {
			fmt.Printf("  %s %s", checkMark, p.Key)
			if p.PinnedVersion != "" {
				fmt.Printf(" (pinned: %s", p.PinnedVersion)
				if p.InstalledVersion != "" && p.InstalledVersion != p.PinnedVersion {
					fmt.Printf(", installed: %s", p.InstalledVersion)
				}
				fmt.Print(")")
			}
			fmt.Println()
		}
		for _, p := range result.PinnedMissing {
			fmt.Printf("  %s %s (pinned: %s, not installed)\n", crossMark, p.Key, p.PinnedVersion)
			hasMissing = true
		}
		fmt.Println()
	}

	// Forked section
	if len(result.ForkedSynced) > 0 || len(result.ForkedMissing) > 0 {
		fmt.Println("FORKED")
		for _, p := range result.ForkedSynced {
			fmt.Printf("  %s %s", checkMark, p.Key)
			if p.InstalledVersion != "" {
				fmt.Printf(" (v%s)", p.InstalledVersion)
			}
			fmt.Println()
		}
		for _, p := range result.ForkedMissing {
			fmt.Printf("  %s %s (not installed)\n", crossMark, p.Key)
			hasMissing = true
		}
		fmt.Println()
	}

	// Untracked section
	if len(result.Untracked) > 0 {
		fmt.Println("UNTRACKED (run 'claude-sync push' to add to config)")
		for _, p := range result.Untracked {
			fmt.Printf("  ? %s\n", p)
		}
		fmt.Println()
	}

	if !hasMissing && len(result.Untracked) == 0 {
		fmt.Println("Everything is in sync.")
	}

	return nil
}

func displayPendingChanges(result *commands.StatusResult) {
	fmt.Println("PENDING (run 'claude-sync approve' to apply, 'claude-sync reject' to discard)")
	if result.PendingChanges.Permissions != nil {
		if len(result.PendingChanges.Permissions.Allow) > 0 {
			fmt.Printf("  %s Permissions allow: %s\n", warningSign, strings.Join(result.PendingChanges.Permissions.Allow, ", "))
		}
		if len(result.PendingChanges.Permissions.Deny) > 0 {
			fmt.Printf("  %s Permissions deny: %s\n", warningSign, strings.Join(result.PendingChanges.Permissions.Deny, ", "))
		}
	}
	if len(result.PendingChanges.MCP) > 0 {
		mcpNames := make([]string, 0, len(result.PendingChanges.MCP))
		for k := range result.PendingChanges.MCP {
			mcpNames = append(mcpNames, k)
		}
		sort.Strings(mcpNames)
		fmt.Printf("  %s MCP servers: %s\n", warningSign, strings.Join(mcpNames, ", "))
	}
	if len(result.PendingChanges.Hooks) > 0 {
		hookNames := make([]string, 0, len(result.PendingChanges.Hooks))
		for k := range result.PendingChanges.Hooks {
			hookNames = append(hookNames, k)
		}
		sort.Strings(hookNames)
		fmt.Printf("  %s Hooks: %s\n", warningSign, strings.Join(hookNames, ", "))
	}
	fmt.Println()
}
