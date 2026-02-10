package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/ruminaider/claude-sync/internal/commands"
	"github.com/ruminaider/claude-sync/internal/config"
	"github.com/ruminaider/claude-sync/internal/paths"
	"github.com/ruminaider/claude-sync/internal/profiles"
	"github.com/spf13/cobra"
)

var profileCmd = &cobra.Command{
	Use:   "profile",
	Short: "Manage sync profiles",
}

var profileListCmd = &cobra.Command{
	Use:   "list",
	Short: "List available profiles",
	RunE: func(cmd *cobra.Command, args []string) error {
		syncDir := paths.SyncDir()

		names, err := profiles.ListProfiles(syncDir)
		if err != nil {
			return err
		}

		if len(names) == 0 {
			fmt.Println("No profiles configured.")
			return nil
		}

		active, err := profiles.ReadActiveProfile(syncDir)
		if err != nil {
			return err
		}

		for _, name := range names {
			p, err := profiles.ReadProfile(syncDir, name)
			if err != nil {
				return err
			}

			summary := profiles.ProfileSummary(p)
			if name == active {
				fmt.Printf("* %s: %s\n", name, summary)
			} else {
				fmt.Printf("  %s: %s\n", name, summary)
			}
		}

		return nil
	},
}

var profileShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Show active profile and resolved plugin list",
	RunE: func(cmd *cobra.Command, args []string) error {
		syncDir := paths.SyncDir()

		active, err := profiles.ReadActiveProfile(syncDir)
		if err != nil {
			return err
		}

		if active == "" {
			fmt.Println("No profile active (using base only)")
		} else {
			p, err := profiles.ReadProfile(syncDir, active)
			if err != nil {
				return err
			}
			fmt.Printf("Active profile: %s\n", active)
			fmt.Printf("  %s\n", profiles.ProfileSummary(p))
		}

		// Read base config to get plugin keys.
		configPath := paths.ConfigFile()
		data, err := os.ReadFile(configPath)
		if err != nil {
			return fmt.Errorf("reading config: %w", err)
		}

		cfg, err := config.Parse(data)
		if err != nil {
			return err
		}

		basePlugins := cfg.AllPluginKeys()

		// Merge with active profile if one is set.
		resolved := basePlugins
		if active != "" {
			p, err := profiles.ReadProfile(syncDir, active)
			if err != nil {
				return err
			}
			resolved = profiles.MergePlugins(basePlugins, p)
		}

		fmt.Println()
		fmt.Println("Resolved plugins:")
		if len(resolved) == 0 {
			fmt.Println("  (none)")
		} else {
			for _, plugin := range resolved {
				fmt.Printf("  - %s\n", plugin)
			}
		}

		return nil
	},
}

var profileSetNone bool

var profileSetCmd = &cobra.Command{
	Use:   "set [name]",
	Short: "Set the active profile",
	Args: func(cmd *cobra.Command, args []string) error {
		if profileSetNone {
			return nil
		}
		if len(args) != 1 {
			return fmt.Errorf("requires a profile name argument (or use --none to deactivate)")
		}
		return nil
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		syncDir := paths.SyncDir()
		claudeDir := paths.ClaudeDir()

		if profileSetNone {
			if err := profiles.DeleteActiveProfile(syncDir); err != nil {
				return err
			}
			fmt.Println("Profile deactivated (using base only)")

			// Re-apply config.
			_, err := commands.Pull(claudeDir, syncDir, true)
			if err != nil {
				return fmt.Errorf("re-applying config: %w", err)
			}
			fmt.Println("Config re-applied.")
			return nil
		}

		name := args[0]

		// Validate that the profile exists.
		names, err := profiles.ListProfiles(syncDir)
		if err != nil {
			return err
		}

		found := false
		for _, n := range names {
			if n == name {
				found = true
				break
			}
		}
		if !found {
			available := strings.Join(names, ", ")
			if available == "" {
				return fmt.Errorf("profile %q not found (no profiles configured)", name)
			}
			return fmt.Errorf("profile %q not found (available: %s)", name, available)
		}

		if err := profiles.WriteActiveProfile(syncDir, name); err != nil {
			return err
		}
		fmt.Printf("Active profile set to %q\n", name)

		// Re-apply config.
		_, err = commands.Pull(claudeDir, syncDir, true)
		if err != nil {
			return fmt.Errorf("re-applying config: %w", err)
		}
		fmt.Println("Config re-applied.")

		return nil
	},
}

func init() {
	profileSetCmd.Flags().BoolVar(&profileSetNone, "none", false, "Deactivate the active profile")

	profileCmd.AddCommand(profileListCmd)
	profileCmd.AddCommand(profileShowCmd)
	profileCmd.AddCommand(profileSetCmd)
}
