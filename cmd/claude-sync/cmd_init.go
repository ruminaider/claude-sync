package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ruminaider/claude-sync/cmd/claude-sync/tui"
	"github.com/ruminaider/claude-sync/internal/commands"
	"github.com/ruminaider/claude-sync/internal/config"
	"github.com/ruminaider/claude-sync/internal/paths"
	"github.com/ruminaider/claude-sync/internal/profiles"
	"github.com/spf13/cobra"
)

var (
	initRemote             string
	initSkipSettings       bool
	initSkipHooks          bool
	initSkipPlugins        bool
	initSkipProfiles       bool
	initSkipClaudeMD       bool
	initSkipPermissions    bool
	initSkipMCP            bool
	initSkipKeybindings    bool
	initSkipCommandsSkills bool
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
		syncDir := paths.SyncDir()

		// Fail if config already exists — use "config update" instead.
		if _, err := os.Stat(filepath.Join(syncDir, "config.yaml")); err == nil {
			return fmt.Errorf("config already exists at %s\nUse \"claude-sync config update\" to modify it", syncDir)
		}

		return runConfigFlow(false, nil, nil)
	},
}

var configUpdateCmd = &cobra.Command{
	Use:   "update",
	Short: "Update existing config from current Claude Code setup",
	RunE: func(cmd *cobra.Command, args []string) error {
		syncDir := paths.SyncDir()

		// Require existing config.
		data, err := os.ReadFile(filepath.Join(syncDir, "config.yaml"))
		if err != nil {
			return fmt.Errorf("no config found at %s\nUse \"claude-sync config create\" to initialize one", syncDir)
		}
		cfg, err := config.Parse(data)
		if err != nil {
			return fmt.Errorf("failed to parse existing config: %w", err)
		}

		var existingProfiles map[string]profiles.Profile
		names, _ := profiles.ListProfiles(syncDir)
		if len(names) > 0 {
			existingProfiles = make(map[string]profiles.Profile)
			for _, name := range names {
				p, err := profiles.ReadProfile(syncDir, name)
				if err == nil {
					existingProfiles[name] = p
				}
			}
		}

		return runConfigFlow(true, &cfg, existingProfiles)
	},
}

// runConfigFlow runs the shared scan → TUI → apply → print flow.
func runConfigFlow(isUpdate bool, existingConfig *config.Config, existingProfiles map[string]profiles.Profile) error {
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
		len(scan.Keybindings) == 0 &&
		(scan.CommandsSkills == nil || len(scan.CommandsSkills.Items) == 0) {
		fmt.Println("No Claude Code configuration found to sync.")
		return nil
	}

	// Phase 2: Launch TUI.
	skip := tui.SkipFlags{
		Plugins:        initSkipPlugins,
		Settings:       initSkipSettings,
		Hooks:          initSkipHooks,
		Permissions:    initSkipPermissions,
		ClaudeMD:       initSkipClaudeMD,
		MCP:            initSkipMCP,
		Keybindings:    initSkipKeybindings,
		CommandsSkills: initSkipCommandsSkills,
	}
	model := tui.NewModel(scan, claudeDir, syncDir, initRemote, initSkipProfiles, skip,
		existingConfig, existingProfiles)
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
	var initResult *commands.InitResult
	if isUpdate {
		initResult, err = commands.Update(*result)
	} else {
		initResult, err = commands.Init(*result)
	}
	if err != nil {
		return err
	}

	// Phase 4: Print results.
	printInitResult(initResult, scan, isUpdate)
	return nil
}

// printInitResult displays the result of the init command.
func printInitResult(result *commands.InitResult, scan *commands.InitScanResult, isUpdate bool) {
	fmt.Println()
	if isUpdate {
		fmt.Println("Updated ~/.claude-sync/")
		fmt.Println("Updated config.yaml from current Claude Code setup")
	} else {
		fmt.Println("Created ~/.claude-sync/")
		fmt.Println("Generated config.yaml from current Claude Code setup")
		fmt.Println("Initialized git repository")
	}

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
	if result.CommandsIncluded > 0 {
		fmt.Printf("  Commands:    %d included\n", result.CommandsIncluded)
	}
	if result.SkillsIncluded > 0 {
		fmt.Printf("  Skills:      %d included\n", result.SkillsIncluded)
	}

	if result.RemotePushed {
		fmt.Println()
		fmt.Println("Pushed to remote")
		fmt.Println()
		fmt.Println("Next steps:")
		fmt.Printf("  On another machine: claude-sync join %s\n", initRemote)
	} else if isUpdate {
		fmt.Println()
		fmt.Println("Next steps:")
		fmt.Println("  1. Review config: cat ~/.claude-sync/config.yaml")
		fmt.Println("  2. Push: claude-sync push -m \"Update config\"")
	} else {
		fmt.Println()
		fmt.Println("Next steps:")
		fmt.Println("  1. Review config: cat ~/.claude-sync/config.yaml")
		fmt.Println("  2. Push: claude-sync push -m \"Initial config\"")
	}
}

// registerConfigFlags adds the shared flags to a config subcommand.
func registerConfigFlags(cmd *cobra.Command) {
	cmd.Flags().StringVarP(&initRemote, "remote", "r", "", "Git remote URL to add as origin and push to")
	cmd.Flags().BoolVar(&initSkipPlugins, "skip-plugins", false, "Skip plugin selection prompt (include all)")
	cmd.Flags().BoolVar(&initSkipSettings, "skip-settings", false, "Don't include settings in sync config")
	cmd.Flags().BoolVar(&initSkipHooks, "skip-hooks", false, "Don't include hooks in sync config")
	cmd.Flags().BoolVar(&initSkipProfiles, "skip-profiles", false, "Skip profile creation prompt")
	cmd.Flags().BoolVar(&initSkipClaudeMD, "skip-claude-md", false, "Don't include CLAUDE.md in sync config")
	cmd.Flags().BoolVar(&initSkipPermissions, "skip-permissions", false, "Don't include permissions in sync config")
	cmd.Flags().BoolVar(&initSkipMCP, "skip-mcp", false, "Don't include MCP servers in sync config")
	cmd.Flags().BoolVar(&initSkipKeybindings, "skip-keybindings", false, "Don't include keybindings in sync config")
	cmd.Flags().BoolVar(&initSkipCommandsSkills, "skip-commands-skills", false, "Don't include commands/skills in sync config")
}

func init() {
	registerConfigFlags(configCreateCmd)
	registerConfigFlags(configUpdateCmd)

	configCmd.AddCommand(configCreateCmd)
	configCmd.AddCommand(configUpdateCmd)
}
