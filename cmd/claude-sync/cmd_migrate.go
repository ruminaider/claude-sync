package main

import (
	"fmt"

	"github.com/charmbracelet/huh"
	"github.com/ruminaider/claude-sync/internal/commands"
	"github.com/ruminaider/claude-sync/internal/paths"
	"github.com/spf13/cobra"
)

var migrateCmd = &cobra.Command{
	Use:   "migrate",
	Short: "Migrate v1 config to v2 (categorized plugins)",
	Long:  "Interactively categorize each plugin as upstream, pinned, or forked and upgrade the config format to v2.",
	RunE: func(cmd *cobra.Command, args []string) error {
		syncDir := paths.SyncDir()

		needed, err := commands.MigrateNeeded(syncDir)
		if err != nil {
			return fmt.Errorf("checking migration status: %w", err)
		}
		if !needed {
			fmt.Println("Config is already v2. No migration needed.")
			return nil
		}

		plugins, err := commands.MigratePlugins(syncDir)
		if err != nil {
			return fmt.Errorf("reading plugins: %w", err)
		}
		if len(plugins) == 0 {
			fmt.Println("No plugins found in config. Nothing to migrate.")
			return nil
		}

		fmt.Println("Migrating config to v2 (categorized plugins)")
		fmt.Println()
		fmt.Println("For each plugin, choose a category:")
		fmt.Println("  upstream  - always pull latest from marketplace")
		fmt.Println("  pinned    - lock to a specific version")
		fmt.Println("  forked    - use a local copy you maintain")
		fmt.Println()

		categories := make(map[string]string, len(plugins))
		versions := make(map[string]string)

		for _, plugin := range plugins {
			var category string
			selectField := huh.NewSelect[string]().
				Title(fmt.Sprintf("Category for %s", plugin)).
				Options(
					huh.NewOption("upstream", "upstream"),
					huh.NewOption("pinned", "pinned"),
					huh.NewOption("forked", "forked"),
				).
				Value(&category)

			if err := huh.NewForm(huh.NewGroup(selectField)).Run(); err != nil {
				return fmt.Errorf("prompt cancelled: %w", err)
			}

			categories[plugin] = category

			if category == "pinned" {
				var version string
				inputField := huh.NewInput().
					Title(fmt.Sprintf("Version for %s", plugin)).
					Placeholder("e.g. 1.0.0").
					Value(&version)

				if err := huh.NewForm(huh.NewGroup(inputField)).Run(); err != nil {
					return fmt.Errorf("prompt cancelled: %w", err)
				}

				if version == "" {
					return fmt.Errorf("version required for pinned plugin %s", plugin)
				}
				versions[plugin] = version
			}
		}

		if err := commands.MigrateApply(syncDir, categories, versions); err != nil {
			return err
		}

		fmt.Println()
		fmt.Println("Config migrated to v2 and committed.")
		fmt.Println("Run 'claude-sync push' to share with your team.")
		return nil
	},
}
