package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/ruminaider/claude-sync/internal/commands"
	"github.com/ruminaider/claude-sync/internal/config"
	"github.com/ruminaider/claude-sync/internal/paths"
	"github.com/ruminaider/claude-sync/internal/profiles"
	"github.com/spf13/cobra"
)

var (
	joinClean        bool
	joinKeepLocal    bool
	joinSkipSettings bool
	joinSkipHooks    bool
	joinReplace      bool
)

var configJoinCmd = &cobra.Command{
	Use:   "join [url]",
	Short: "Join existing config repo",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		var repoURL string
		if len(args) > 0 {
			repoURL = args[0]
		} else {
			err := huh.NewForm(
				huh.NewGroup(
					huh.NewInput().
						Title("Config repo URL:").
						Placeholder("git@github.com:org/claude-code-config.git").
						Value(&repoURL),
				),
			).Run()
			if err != nil {
				return fmt.Errorf("cancelled")
			}
			if repoURL == "" {
				return fmt.Errorf("URL is required")
			}
		}
		syncDir := paths.SyncDir()
		claudeDir := paths.ClaudeDir()

		// If --replace is set and sync dir exists, confirm and replace.
		if joinReplace {
			if _, statErr := os.Stat(syncDir); statErr == nil {
				subCount := countSubscriptions(syncDir)
				warning := "This will remove your current config"
				if subCount > 0 {
					warning += fmt.Sprintf(" and %d subscription(s)", subCount)
				}
				var confirm bool
				confirmErr := huh.NewForm(
					huh.NewGroup(
						huh.NewConfirm().
							Title(warning + ". Continue?").
							Affirmative("Yes, replace").
							Negative("Cancel").
							Value(&confirm),
					),
				).Run()
				if confirmErr != nil || !confirm {
					return fmt.Errorf("cancelled")
				}
			}
		}

		fmt.Printf("Cloning config from %s...\n", repoURL)

		var result *commands.JoinResult
		var err error
		if joinReplace {
			result, err = commands.JoinReplace(repoURL, claudeDir, syncDir)
		} else {
			result, err = commands.Join(repoURL, claudeDir, syncDir)
		}

		var alreadyErr *commands.AlreadyJoinedError
		var subscribeErr *commands.SubscribeNeededError
		if errors.As(err, &alreadyErr) {
			fmt.Println("Already joined this config. Running pull...")
			pullResult, pullErr := commands.Pull(claudeDir, syncDir, false)
			if pullErr != nil {
				return fmt.Errorf("pull failed: %w", pullErr)
			}
			printPullResult(pullResult)
			return nil
		}
		if errors.As(err, &subscribeErr) {
			fmt.Printf("You already have a config repo. To add items from this source, run:\n\n")
			fmt.Printf("  claude-sync subscribe %s\n\n", subscribeErr.URL)
			fmt.Println("Or use 'claude-sync config join --replace' to replace your current config.")
			return nil
		}
		if err != nil {
			return err
		}

		fmt.Println("✓ Cloned config repo to ~/.claude-sync/")

		// Show what the config contains.
		if result.HasSettings {
			fmt.Printf("  Config includes settings: %s\n", strings.Join(result.SettingsKeys, ", "))
		}
		if result.HasHooks {
			fmt.Printf("  Config includes hooks: %s\n", strings.Join(result.HookNames, ", "))
		}

		// Category selection: prompt user about which categories to apply on this machine.
		var skipCategories []string

		if joinSkipSettings && result.HasSettings {
			skipCategories = append(skipCategories, string(config.CategorySettings))
		}
		if joinSkipHooks && result.HasHooks {
			skipCategories = append(skipCategories, string(config.CategoryHooks))
		}

		// Only prompt if there are categories to choose from and no skip flags cover them all.
		hasPromptableCategories := (result.HasSettings && !joinSkipSettings) || (result.HasHooks && !joinSkipHooks)
		if hasPromptableCategories && !joinSkipSettings && !joinSkipHooks {
			selected, err := promptCategorySelection(result)
			if err != nil {
				os.RemoveAll(syncDir)
				return fmt.Errorf("cancelled")
			}
			// Determine what was deselected.
			selectedSet := make(map[string]bool)
			for _, s := range selected {
				selectedSet[s] = true
			}
			if result.HasSettings && !selectedSet[string(config.CategorySettings)] {
				skipCategories = append(skipCategories, string(config.CategorySettings))
			}
			if result.HasHooks && !selectedSet[string(config.CategoryHooks)] {
				skipCategories = append(skipCategories, string(config.CategoryHooks))
			}
		}

		// Write skip preferences if any categories were deselected.
		if len(skipCategories) > 0 {
			prefs := config.DefaultUserPreferences()

			// Try to read existing prefs first.
			prefsPath := filepath.Join(syncDir, "user-preferences.yaml")
			if prefsData, err := os.ReadFile(prefsPath); err == nil {
				if parsed, err := config.ParseUserPreferences(prefsData); err == nil {
					prefs = parsed
				}
			}

			prefs.Sync.Skip = skipCategories
			data, err := config.MarshalUserPreferences(prefs)
			if err != nil {
				return fmt.Errorf("writing user preferences: %w", err)
			}
			if err := os.WriteFile(prefsPath, data, 0644); err != nil {
				return fmt.Errorf("writing user preferences: %w", err)
			}
			fmt.Printf("  Skipping: %s (saved to user-preferences.yaml)\n", strings.Join(skipCategories, ", "))
		}

		// Profile activation.
		if result.HasProfiles {
			fmt.Println()
			fmt.Printf("Found %d profile(s) available:\n", len(result.ProfileNames))
			for _, name := range result.ProfileNames {
				p, err := profiles.ReadProfile(syncDir, name)
				if err == nil {
					fmt.Printf("  %s: %s\n", capitalize(name), profiles.ProfileSummary(p))
				}
			}

			options := make([]huh.Option[string], 0, len(result.ProfileNames)+1)
			for _, name := range result.ProfileNames {
				options = append(options, huh.NewOption(capitalize(name), name))
			}
			options = append(options, huh.NewOption("No — keep base only", ""))

			var profileChoice string
			fmt.Println()
			err := huh.NewForm(
				huh.NewGroup(
					huh.NewSelect[string]().
						Title("Activate a profile on this machine?").
						Options(options...).
						Value(&profileChoice),
				),
			).Run()
			if err == nil && profileChoice != "" {
				if err := profiles.WriteActiveProfile(syncDir, profileChoice); err != nil {
					return fmt.Errorf("writing active profile: %w", err)
				}
				fmt.Printf("  Activated profile: %s\n", profileChoice)
			}
		}

		// Local plugin cleanup.
		if len(result.LocalOnly) > 0 && !joinKeepLocal {
			fmt.Printf("\nFound %d locally installed plugin(s) not in the remote config:\n", len(result.LocalOnly))
			for _, p := range result.LocalOnly {
				fmt.Printf("  • %s (%s)\n", p.Key, p.Scope)
			}

			var toRemove []commands.LocalPlugin

			if joinClean {
				toRemove = result.LocalOnly
			} else {
				toRemove, err = promptLocalPluginCleanup(result.LocalOnly)
				if err != nil {
					os.RemoveAll(syncDir)
					return fmt.Errorf("cancelled")
				}
			}

			if len(toRemove) > 0 {
				results := commands.JoinCleanup(toRemove)
				for _, r := range results {
					if r.Err == nil {
						fmt.Printf("  ✓ Removed %s\n", r.Plugin)
					} else {
						fmt.Printf("  ✗ Failed to remove %s: %v\n", r.Plugin, r.Err)
					}
				}
			}
		}

		// Automatically apply the config.
		fmt.Println()
		fmt.Println("Applying config...")
		pullResult, pullErr := commands.Pull(claudeDir, syncDir, false)
		if pullErr != nil {
			return fmt.Errorf("pull failed: %w", pullErr)
		}

		printPullResult(pullResult)
		return nil
	},
}

// countSubscriptions returns the number of subscriptions in the sync dir.
func countSubscriptions(syncDir string) int {
	cfgData, err := os.ReadFile(filepath.Join(syncDir, "config.yaml"))
	if err != nil {
		return 0
	}
	cfg, err := config.Parse(cfgData)
	if err != nil {
		return 0
	}
	return len(cfg.Subscriptions)
}

// promptCategorySelection asks the user which sync categories to apply.
func promptCategorySelection(result *commands.JoinResult) ([]string, error) {
	var options []huh.Option[string]
	var defaults []string

	if result.HasSettings {
		label := fmt.Sprintf("Settings (%s)", strings.Join(result.SettingsKeys, ", "))
		options = append(options, huh.NewOption(label, string(config.CategorySettings)))
		defaults = append(defaults, string(config.CategorySettings))
	}
	if result.HasHooks {
		label := fmt.Sprintf("Hooks (%s)", strings.Join(result.HookNames, ", "))
		options = append(options, huh.NewOption(label, string(config.CategoryHooks)))
		defaults = append(defaults, string(config.CategoryHooks))
	}

	if len(options) == 0 {
		return nil, nil
	}

	selected := defaults
	fmt.Println()
	err := huh.NewForm(
		huh.NewGroup(
			huh.NewMultiSelect[string]().
				Title("Sync these categories on this machine:").
				Options(options...).
				Value(&selected),
		),
	).Run()
	if err != nil {
		return nil, err
	}

	return selected, nil
}

// promptLocalPluginCleanup asks the user what to do with local-only plugins.
func promptLocalPluginCleanup(plugins []commands.LocalPlugin) ([]commands.LocalPlugin, error) {
	var choice string
	fmt.Println()
	err := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("What would you like to do with these plugins?").
				Options(
					huh.NewOption("Keep all", "keep"),
					huh.NewOption("Choose which to remove", "some"),
					huh.NewOption("Remove all", "all"),
				).
				Value(&choice),
		),
	).Run()
	if err != nil {
		return nil, err
	}

	switch choice {
	case "all":
		// Confirm before bulk removal.
		var confirm bool
		err := huh.NewForm(
			huh.NewGroup(
				huh.NewConfirm().
					Title(fmt.Sprintf("Remove all %d local plugin(s)?", len(plugins))).
					Affirmative("Yes, remove all").
					Negative("Cancel").
					Value(&confirm),
			),
		).Run()
		if err != nil || !confirm {
			return nil, nil
		}
		return plugins, nil
	case "keep":
		return nil, nil
	case "some":
		// Build key list for the picker, then map selections back.
		keys := make([]string, len(plugins))
		byKey := make(map[string]commands.LocalPlugin)
		for i, p := range plugins {
			keys[i] = p.Key
			byKey[p.Key] = p
		}

		selected, err := runPicker("Select plugins to remove:", keys)
		if err != nil {
			return nil, err
		}

		var result []commands.LocalPlugin
		for _, k := range selected {
			result = append(result, byKey[k])
		}
		return result, nil
	}

	return nil, nil
}

func init() {
	configJoinCmd.Flags().BoolVar(&joinClean, "clean", false, "Automatically remove local-only plugins not in the remote config")
	configJoinCmd.Flags().BoolVar(&joinKeepLocal, "keep-local", false, "Keep all locally installed plugins without prompting")
	configJoinCmd.Flags().BoolVar(&joinSkipSettings, "skip-settings", false, "Don't apply settings from the remote config")
	configJoinCmd.Flags().BoolVar(&joinSkipHooks, "skip-hooks", false, "Don't apply hooks from the remote config")
	configJoinCmd.Flags().BoolVar(&joinReplace, "replace", false, "Replace existing config repo with a new one")

	configCmd.AddCommand(configJoinCmd)
}
