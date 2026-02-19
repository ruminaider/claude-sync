package main

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ruminaider/claude-sync/cmd/claude-sync/tui"
	"github.com/ruminaider/claude-sync/internal/commands"
	"github.com/ruminaider/claude-sync/internal/paths"
	"github.com/spf13/cobra"
)

var (
	initRemote          string
	initSkipSettings    bool
	initSkipHooks       bool
	initSkipPlugins     bool
	initSkipProfiles    bool
	initSkipClaudeMD    bool
	initSkipPermissions bool
	initSkipMCP         bool
	initSkipKeybindings bool
)

// capitalize returns the string with its first letter uppercased.
func capitalize(s string) string {
	if s == "" {
		return ""
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

var configCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create new config from current Claude Code setup",
	RunE: func(cmd *cobra.Command, args []string) error {
		claudeDir := paths.ClaudeDir()
		syncDir := paths.SyncDir()

		// Phase 1: Scan.
		scan, err := commands.InitScan(claudeDir)
		if err != nil {
			return err
		}

		// Check if anything was found.
		if len(scan.PluginKeys) == 0 &&
			len(scan.Settings) == 0 &&
			len(scan.Hooks) == 0 &&
			len(scan.Permissions.Allow) == 0 &&
			len(scan.Permissions.Deny) == 0 &&
			scan.ClaudeMDContent == "" &&
			len(scan.MCP) == 0 &&
			len(scan.Keybindings) == 0 {
			fmt.Println("No Claude Code configuration found to sync.")
			return nil
		}

		// Phase 2: Launch TUI.
		skip := tui.SkipFlags{
			Plugins:     initSkipPlugins,
			Settings:    initSkipSettings,
			Hooks:       initSkipHooks,
			Permissions: initSkipPermissions,
			ClaudeMD:    initSkipClaudeMD,
			MCP:         initSkipMCP,
			Keybindings: initSkipKeybindings,
		}
		model := tui.NewModel(scan, claudeDir, syncDir, initRemote, initSkipProfiles, skip)
		p := tea.NewProgram(model, tea.WithAltScreen())
		finalModel, err := p.Run()
		if err != nil {
			return err
		}

		result := finalModel.(tui.Model).Result
		if result == nil {
			// User cancelled.
			return nil
		}

		// Phase 3: Apply.
		initResult, err := commands.Init(*result)
		if err != nil {
			return err
		}

		// Phase 4: Print results.
		printInitResult(initResult, scan)
		return nil
	},
}

// printInitResult displays the result of the init command.
func printInitResult(result *commands.InitResult, scan *commands.InitScanResult) {
	fmt.Println()
	fmt.Println("Created ~/.claude-sync/")
	fmt.Println("Generated config.yaml from current Claude Code setup")
	fmt.Println("Initialized git repository")

	if len(result.Upstream) > 0 {
		fmt.Printf("\n  Upstream:    %d plugin(s)\n", len(result.Upstream))
	}
	if len(result.AutoForked) > 0 {
		fmt.Printf("  Auto-forked: %d plugin(s) (non-portable marketplace)\n", len(result.AutoForked))
		for _, p := range result.AutoForked {
			fmt.Printf("    -> %s\n", p)
		}
	}
	if len(result.ExcludedPlugins) > 0 {
		fmt.Printf("  Excluded:    %d plugin(s) (not selected)\n", len(result.ExcludedPlugins))
	}
	if len(result.IncludedSettings) > 0 {
		fmt.Printf("  Settings:    %s\n", strings.Join(result.IncludedSettings, ", "))
	}
	if len(result.IncludedHooks) > 0 {
		fmt.Printf("  Hooks:       %s\n", strings.Join(result.IncludedHooks, ", "))
	}
	if len(result.ProfileNames) > 0 {
		fmt.Printf("  Profiles:    %s\n", strings.Join(result.ProfileNames, ", "))
	}
	if result.ActiveProfile != "" {
		fmt.Printf("  Active:      %s\n", result.ActiveProfile)
	}
	if result.PermissionsIncluded {
		allowCount := len(scan.Permissions.Allow)
		denyCount := len(scan.Permissions.Deny)
		fmt.Printf("  Permissions: %d allow, %d deny\n", allowCount, denyCount)
	}
	if len(result.ClaudeMDFragments) > 0 {
		fmt.Printf("  CLAUDE.md:   %d fragment(s)\n", len(result.ClaudeMDFragments))
	}
	if len(result.MCPIncluded) > 0 {
		fmt.Printf("  MCP servers: %s\n", strings.Join(result.MCPIncluded, ", "))
	}
	if result.KeybindingsIncluded {
		fmt.Println("  Keybindings: included")
	}

	if result.RemotePushed {
		fmt.Println()
		fmt.Println("Pushed to remote")
		fmt.Println()
		fmt.Println("Next steps:")
		fmt.Printf("  On another machine: claude-sync join %s\n", initRemote)
	} else {
		fmt.Println()
		fmt.Println("Next steps:")
		fmt.Println("  1. Review config: cat ~/.claude-sync/config.yaml")
		fmt.Println("  2. Push: claude-sync push -m \"Initial config\"")
	}
}

var initAliasCmd = &cobra.Command{
	Use:        "init",
	Short:      "Create new config from current Claude Code setup",
	Hidden:     true,
	Deprecated: "Use 'claude-sync config create' instead.",
	RunE:       configCreateCmd.RunE,
}

func init() {
	configCreateCmd.Flags().StringVarP(&initRemote, "remote", "r", "", "Git remote URL to add as origin and push to")
	configCreateCmd.Flags().BoolVar(&initSkipPlugins, "skip-plugins", false, "Skip plugin selection prompt (include all)")
	configCreateCmd.Flags().BoolVar(&initSkipSettings, "skip-settings", false, "Don't include settings in sync config")
	configCreateCmd.Flags().BoolVar(&initSkipHooks, "skip-hooks", false, "Don't include hooks in sync config")
	configCreateCmd.Flags().BoolVar(&initSkipProfiles, "skip-profiles", false, "Skip profile creation prompt")
	configCreateCmd.Flags().BoolVar(&initSkipClaudeMD, "skip-claude-md", false, "Don't include CLAUDE.md in sync config")
	configCreateCmd.Flags().BoolVar(&initSkipPermissions, "skip-permissions", false, "Don't include permissions in sync config")
	configCreateCmd.Flags().BoolVar(&initSkipMCP, "skip-mcp", false, "Don't include MCP servers in sync config")
	configCreateCmd.Flags().BoolVar(&initSkipKeybindings, "skip-keybindings", false, "Don't include keybindings in sync config")

	// Copy flags to alias for full backward compatibility.
	initAliasCmd.Flags().StringVarP(&initRemote, "remote", "r", "", "Git remote URL to add as origin and push to")
	initAliasCmd.Flags().BoolVar(&initSkipPlugins, "skip-plugins", false, "Skip plugin selection prompt (include all)")
	initAliasCmd.Flags().BoolVar(&initSkipSettings, "skip-settings", false, "Don't include settings in sync config")
	initAliasCmd.Flags().BoolVar(&initSkipHooks, "skip-hooks", false, "Don't include hooks in sync config")
	initAliasCmd.Flags().BoolVar(&initSkipProfiles, "skip-profiles", false, "Skip profile creation prompt")
	initAliasCmd.Flags().BoolVar(&initSkipClaudeMD, "skip-claude-md", false, "Don't include CLAUDE.md in sync config")
	initAliasCmd.Flags().BoolVar(&initSkipPermissions, "skip-permissions", false, "Don't include permissions in sync config")
	initAliasCmd.Flags().BoolVar(&initSkipMCP, "skip-mcp", false, "Don't include MCP servers in sync config")
	initAliasCmd.Flags().BoolVar(&initSkipKeybindings, "skip-keybindings", false, "Don't include keybindings in sync config")

	configCmd.AddCommand(configCreateCmd)
}
