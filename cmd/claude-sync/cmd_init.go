package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/ruminaider/claude-sync/internal/commands"
	"github.com/ruminaider/claude-sync/internal/paths"
	"github.com/ruminaider/claude-sync/internal/profiles"
	"github.com/spf13/cobra"
)

var (
	initRemote       string
	initSkipSettings bool
	initSkipHooks    bool
	initSkipPlugins  bool
	initSkipProfiles bool
)

type initStep int

const (
	stepPluginStrategy  initStep = iota
	stepPluginPicker
	stepSettings
	stepHookStrategy
	stepHookPicker
	stepProfilePrompt
	stepProfileName
	stepProfilePlugins
	stepProfileSettings
	stepProfileHooks
	stepProfileLoop
	stepProfileActivate
	stepDone
)

// capitalize returns the string with its first letter uppercased.
func capitalize(s string) string {
	if s == "" {
		return ""
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Create new config from current Claude Code setup",
	RunE: func(cmd *cobra.Command, args []string) error {
		claudeDir := paths.ClaudeDir()
		syncDir := paths.SyncDir()

		// Phase 1: Scan what's available.
		scan, err := commands.InitScan(claudeDir)
		if err != nil {
			return err
		}

		// Show scan results.
		if len(scan.PluginKeys) > 0 {
			fmt.Printf("Found %d plugin(s)\n", len(scan.PluginKeys))
		}
		if len(scan.Settings) > 0 {
			keys := make([]string, 0, len(scan.Settings))
			for k := range scan.Settings {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			fmt.Printf("Found settings: %s\n", strings.Join(keys, ", "))
		}
		if len(scan.Hooks) > 0 {
			// Build display strings: "PreCompact: bd prime"
			hookDisplays := make([]string, 0, len(scan.Hooks))
			for k, v := range scan.Hooks {
				cmd := commands.ExtractHookCommand(v)
				if cmd != "" {
					hookDisplays = append(hookDisplays, fmt.Sprintf("%s (%s)", k, cmd))
				} else {
					hookDisplays = append(hookDisplays, k)
				}
			}
			sort.Strings(hookDisplays)
			fmt.Printf("Found hooks: %s\n", strings.Join(hookDisplays, ", "))
		}

		// Phase 2: Interactive prompts with go-back navigation.
		var includePlugins []string                 // nil = all
		includeSettings := true
		var includeHooks map[string]json.RawMessage // nil = all

		// Profile-related variables.
		var createdProfiles map[string]profiles.Profile // name -> profile
		var activeProfile string
		var currentProfileName string
		var currentProfile profiles.Profile

		// Store all plugin keys for the profile picker.
		allPluginKeys := scan.PluginKeys

		// Determine the starting step based on flags and data availability.
		hasPlugins := len(scan.PluginKeys) > 0 && !initSkipPlugins
		hasSettings := len(scan.Settings) > 0 && !initSkipSettings
		hasHooks := len(scan.Hooks) > 0 && !initSkipHooks

		if initSkipSettings {
			includeSettings = false
		}
		if initSkipHooks {
			includeHooks = map[string]json.RawMessage{} // empty = none
		}

		step := stepDone
		if hasHooks {
			step = stepHookStrategy
		}
		if hasSettings {
			step = stepSettings
		}
		if hasPlugins {
			step = stepPluginStrategy
		}
		firstStep := step

		for step != stepDone {
			switch step {
			case stepPluginStrategy:
				var choice string
				fmt.Println()
				err := huh.NewForm(
					huh.NewGroup(
						huh.NewSelect[string]().
							Title(fmt.Sprintf("Include all %d plugins in sync?", len(scan.PluginKeys))).
							Options(
								huh.NewOption("Share all (Recommended)", "all"),
								huh.NewOption("Choose which to share", "some"),
								huh.NewOption("Don't share any plugins", "none"),
							).
							Value(&choice),
					),
				).Run()
				if err != nil {
					if errors.Is(err, huh.ErrUserAborted) {
						if step == firstStep {
							return err
						}
						// Can't go back further from first step.
						return err
					}
					return err
				}

				switch choice {
				case "all":
					includePlugins = nil // nil = all
					if hasSettings {
						step = stepSettings
					} else if hasHooks {
						step = stepHookStrategy
					} else {
						step = stepDone
					}
				case "none":
					includePlugins = []string{} // empty = none
					if hasSettings {
						step = stepSettings
					} else if hasHooks {
						step = stepHookStrategy
					} else {
						step = stepDone
					}
				case "some":
					step = stepPluginPicker
				}

			case stepPluginPicker:
				// Build sections from upstream and auto-forked.
				var sections []pickerSection
				if len(scan.Upstream) > 0 {
					sections = append(sections, pickerSection{
						Header: fmt.Sprintf("Upstream (%d)", len(scan.Upstream)),
						Items:  scan.Upstream,
					})
				}
				if len(scan.AutoForked) > 0 {
					sections = append(sections, pickerSection{
						Header: fmt.Sprintf("Auto-forked (%d)", len(scan.AutoForked)),
						Items:  scan.AutoForked,
					})
				}

				selected, err := runPickerWithSections("Select plugins to share:", sections)
				if err != nil {
					if errors.Is(err, huh.ErrUserAborted) {
						step = stepPluginStrategy
						continue
					}
					return err
				}
				includePlugins = selected
				if hasSettings {
					step = stepSettings
				} else if hasHooks {
					step = stepHookStrategy
				} else {
					step = stepDone
				}

			case stepSettings:
				keys := make([]string, 0, len(scan.Settings))
				for k := range scan.Settings {
					keys = append(keys, k)
				}
				sort.Strings(keys)

				var confirm bool
				fmt.Println()
				err := huh.NewForm(
					huh.NewGroup(
						huh.NewConfirm().
							Title(fmt.Sprintf("Include settings in sync? (%s)", strings.Join(keys, ", "))).
							Affirmative("Yes").
							Negative("No").
							Value(&confirm),
					),
				).Run()
				if err != nil {
					if errors.Is(err, huh.ErrUserAborted) {
						if step == firstStep {
							return err
						}
						if hasPlugins {
							step = stepPluginStrategy
							continue
						}
						return err
					}
					return err
				}
				includeSettings = confirm
				if hasHooks {
					step = stepHookStrategy
				} else {
					step = stepDone
				}

			case stepHookStrategy:
				// Build sorted hook names with commands for display.
				hookNames := make([]string, 0, len(scan.Hooks))
				for k := range scan.Hooks {
					hookNames = append(hookNames, k)
				}
				sort.Strings(hookNames)

				hookSummary := make([]string, 0, len(hookNames))
				for _, name := range hookNames {
					cmd := commands.ExtractHookCommand(scan.Hooks[name])
					if cmd != "" {
						hookSummary = append(hookSummary, fmt.Sprintf("%s (%s)", name, cmd))
					} else {
						hookSummary = append(hookSummary, name)
					}
				}

				var choice string
				fmt.Println()
				err := huh.NewForm(
					huh.NewGroup(
						huh.NewSelect[string]().
							Title(fmt.Sprintf("Sync hooks? (Found: %s)", strings.Join(hookSummary, ", "))).
							Options(
								huh.NewOption("Share all", "all"),
								huh.NewOption("Choose which to share", "some"),
								huh.NewOption("Don't share hooks", "none"),
							).
							Value(&choice),
					),
				).Run()
				if err != nil {
					if errors.Is(err, huh.ErrUserAborted) {
						if step == firstStep {
							return err
						}
						if hasSettings {
							step = stepSettings
						} else if hasPlugins {
							step = stepPluginStrategy
						} else {
							return err
						}
						continue
					}
					return err
				}

				switch choice {
				case "all":
					includeHooks = nil // nil = all
					step = stepDone
				case "none":
					includeHooks = map[string]json.RawMessage{} // empty = none
					step = stepDone
				case "some":
					step = stepHookPicker
				}

			case stepHookPicker:
				// Build display strings with commands for the picker.
				hookNames := make([]string, 0, len(scan.Hooks))
				for k := range scan.Hooks {
					hookNames = append(hookNames, k)
				}
				sort.Strings(hookNames)

				displayItems := make([]string, 0, len(hookNames))
				displayToName := make(map[string]string)
				for _, name := range hookNames {
					cmd := commands.ExtractHookCommand(scan.Hooks[name])
					display := name
					if cmd != "" {
						display = fmt.Sprintf("%s: %s", name, cmd)
					}
					displayItems = append(displayItems, display)
					displayToName[display] = name
				}

				selected, err := runPicker("Select hooks to share:", displayItems)
				if err != nil {
					if errors.Is(err, huh.ErrUserAborted) {
						step = stepHookStrategy
						continue
					}
					return err
				}
				includeHooks = make(map[string]json.RawMessage)
				for _, display := range selected {
					name := displayToName[display]
					includeHooks[name] = scan.Hooks[name]
				}
				step = stepDone
			}
		}

		// Compute basePluginKeys from includePlugins (what the user chose for base).
		// If includePlugins is nil, all scan.PluginKeys are base.
		// If includePlugins is []string{}, none are base.
		// Otherwise, includePlugins IS the base.
		var basePluginKeys []string
		if includePlugins == nil {
			basePluginKeys = allPluginKeys
		} else {
			basePluginKeys = includePlugins
		}

		// Profile creation loop — only show if there are plugins and profiles aren't skipped.
		if !initSkipProfiles && len(allPluginKeys) > 0 {
			step = stepProfilePrompt
		}

		for step != stepDone {
			switch step {
			case stepProfilePrompt:
				// Show a summary of base config.
				fmt.Println()
				fmt.Printf("Base configured: %d plugin(s)", len(basePluginKeys))
				if includeSettings && len(scan.Settings) > 0 {
					settingKeys := make([]string, 0, len(scan.Settings))
					for k := range scan.Settings {
						settingKeys = append(settingKeys, k)
					}
					sort.Strings(settingKeys)
					fmt.Printf(", settings: %s", strings.Join(settingKeys, ", "))
				}
				if includeHooks == nil || len(includeHooks) > 0 {
					hookCount := len(scan.Hooks)
					if includeHooks != nil {
						hookCount = len(includeHooks)
					}
					if hookCount > 0 {
						fmt.Printf(", %d hook(s)", hookCount)
					}
				}
				fmt.Println()

				var wantProfiles bool
				err := huh.NewForm(
					huh.NewGroup(
						huh.NewConfirm().
							Title("Set up profiles? (e.g., work/personal layers on top of base)").
							Affirmative("Yes").
							Negative("No").
							Value(&wantProfiles),
					),
				).Run()
				if err != nil {
					if errors.Is(err, huh.ErrUserAborted) {
						// Escaping from the profile prompt skips profiles entirely.
						step = stepDone
						continue
					}
					return err
				}

				if !wantProfiles {
					step = stepDone
				} else {
					createdProfiles = make(map[string]profiles.Profile)
					step = stepProfileName
				}

			case stepProfileName:
				// Build preset options, filtering out already-used names.
				type nameOption struct {
					label string
					value string
				}
				presets := []nameOption{
					{"Work", "work"},
					{"Personal", "personal"},
				}

				var options []huh.Option[string]
				for _, p := range presets {
					if _, used := createdProfiles[p.value]; !used {
						options = append(options, huh.NewOption(p.label, p.value))
					}
				}
				options = append(options, huh.NewOption("Custom name...", "_custom"))
				if len(createdProfiles) > 0 {
					options = append(options, huh.NewOption("Done creating profiles", "_done"))
				}

				var nameChoice string
				fmt.Println()
				err := huh.NewForm(
					huh.NewGroup(
						huh.NewSelect[string]().
							Title("Profile name:").
							Options(options...).
							Value(&nameChoice),
					),
				).Run()
				if err != nil {
					if errors.Is(err, huh.ErrUserAborted) {
						if len(createdProfiles) == 0 {
							step = stepProfilePrompt
						} else {
							step = stepDone
						}
						continue
					}
					return err
				}

				switch nameChoice {
				case "_done":
					step = stepDone
				case "_custom":
					var customName string
					err := huh.NewForm(
						huh.NewGroup(
							huh.NewInput().
								Title("Profile name:").
								Value(&customName),
						),
					).Run()
					if err != nil {
						if errors.Is(err, huh.ErrUserAborted) {
							// Go back to the name selection.
							continue
						}
						return err
					}
					customName = strings.TrimSpace(strings.ToLower(customName))
					if customName == "" {
						// Empty name, try again.
						continue
					}
					if _, used := createdProfiles[customName]; used {
						fmt.Printf("  Profile %q already exists, choose another name.\n", customName)
						continue
					}
					currentProfileName = customName
					currentProfile = profiles.Profile{}
					step = stepProfilePlugins
				default:
					currentProfileName = nameChoice
					currentProfile = profiles.Profile{}
					step = stepProfilePlugins
				}

			case stepProfilePlugins:
				// Build two sections: "Base" and "Not in base".
				baseSet := make(map[string]bool, len(basePluginKeys))
				for _, k := range basePluginKeys {
					baseSet[k] = true
				}

				var baseItems []string
				var nonBaseItems []string
				for _, k := range allPluginKeys {
					if baseSet[k] {
						baseItems = append(baseItems, k)
					} else {
						nonBaseItems = append(nonBaseItems, k)
					}
				}

				var sections []pickerSection
				if len(baseItems) > 0 {
					sections = append(sections, pickerSection{
						Header: fmt.Sprintf("Base (%d)", len(baseItems)),
						Items:  baseItems,
					})
				}
				if len(nonBaseItems) > 0 {
					sections = append(sections, pickerSection{
						Header: fmt.Sprintf("Not in base (%d)", len(nonBaseItems)),
						Items:  nonBaseItems,
					})
				}

				// Pre-select only base items.
				preSelected := make(map[string]bool, len(baseItems))
				for _, k := range baseItems {
					preSelected[k] = true
				}

				selected, err := runPickerWithPreSelected(
					fmt.Sprintf("Select plugins for %q:", currentProfileName),
					sections,
					preSelected,
				)
				if err != nil {
					if errors.Is(err, huh.ErrUserAborted) {
						step = stepProfileName
						continue
					}
					return err
				}

				// Compute the diff: adds and removes relative to base.
				selectedSet := make(map[string]bool, len(selected))
				for _, s := range selected {
					selectedSet[s] = true
				}

				var adds []string
				for _, s := range selected {
					if !baseSet[s] {
						adds = append(adds, s)
					}
				}
				var removes []string
				for _, b := range basePluginKeys {
					if !selectedSet[b] {
						removes = append(removes, b)
					}
				}

				currentProfile.Plugins = profiles.ProfilePlugins{Add: adds, Remove: removes}
				step = stepProfileSettings

			case stepProfileSettings:
				// Check if base has settings. If not, skip to hooks.
				if !includeSettings || len(scan.Settings) == 0 {
					step = stepProfileHooks
					continue
				}

				var overrideModel bool
				fmt.Println()
				err := huh.NewForm(
					huh.NewGroup(
						huh.NewConfirm().
							Title(fmt.Sprintf("Override model for %q?", currentProfileName)).
							Affirmative("Yes").
							Negative("No").
							Value(&overrideModel),
					),
				).Run()
				if err != nil {
					if errors.Is(err, huh.ErrUserAborted) {
						step = stepProfilePlugins
						continue
					}
					return err
				}

				if overrideModel {
					var modelValue string
					err := huh.NewForm(
						huh.NewGroup(
							huh.NewInput().
								Title("Model value:").
								Value(&modelValue),
						),
					).Run()
					if err != nil {
						if errors.Is(err, huh.ErrUserAborted) {
							// Go back to the override question.
							continue
						}
						return err
					}
					modelValue = strings.TrimSpace(modelValue)
					if modelValue != "" {
						currentProfile.Settings = map[string]any{"model": modelValue}
					}
				}

				step = stepProfileHooks

			case stepProfileHooks:
				// Check if base has hooks. If not, skip to profile loop.
				if includeHooks != nil && len(includeHooks) == 0 {
					// Base has no hooks.
					step = stepProfileLoop
					continue
				}

				var overrideHooks string
				fmt.Println()
				err := huh.NewForm(
					huh.NewGroup(
						huh.NewSelect[string]().
							Title(fmt.Sprintf("Override hooks for %q?", currentProfileName)).
							Options(
								huh.NewOption("Keep base hooks", "keep"),
								huh.NewOption("Customize", "customize"),
							).
							Value(&overrideHooks),
					),
				).Run()
				if err != nil {
					if errors.Is(err, huh.ErrUserAborted) {
						step = stepProfileSettings
						continue
					}
					return err
				}

				if overrideHooks == "customize" {
					// Determine base hooks.
					baseHooks := scan.Hooks
					if includeHooks != nil {
						baseHooks = includeHooks
					}

					hookNames := make([]string, 0, len(baseHooks))
					for k := range baseHooks {
						hookNames = append(hookNames, k)
					}
					sort.Strings(hookNames)

					// Build display items with commands.
					displayItems := make([]string, 0, len(hookNames))
					displayToName := make(map[string]string)
					for _, name := range hookNames {
						cmd := commands.ExtractHookCommand(baseHooks[name])
						display := name
						if cmd != "" {
							display = fmt.Sprintf("%s: %s", name, cmd)
						}
						displayItems = append(displayItems, display)
						displayToName[display] = name
					}

					selected, err := runPicker(
						fmt.Sprintf("Select hooks for %q:", currentProfileName),
						displayItems,
					)
					if err != nil {
						if errors.Is(err, huh.ErrUserAborted) {
							// Go back to the customize question.
							continue
						}
						return err
					}

					// Compute removes: hooks in base but not selected.
					selectedNames := make(map[string]bool, len(selected))
					for _, display := range selected {
						name := displayToName[display]
						selectedNames[name] = true
					}

					var hookRemoves []string
					for _, name := range hookNames {
						if !selectedNames[name] {
							hookRemoves = append(hookRemoves, name)
						}
					}
					if len(hookRemoves) > 0 {
						currentProfile.Hooks = profiles.ProfileHooks{Remove: hookRemoves}
					}
				}

				step = stepProfileLoop

			case stepProfileLoop:
				// Save the completed profile and reset for next one.
				createdProfiles[currentProfileName] = currentProfile
				fmt.Printf("  ✓ Profile %q configured\n", currentProfileName)
				currentProfile = profiles.Profile{}
				currentProfileName = ""

				// Go to stepProfileName which will show "Done creating profiles" option.
				step = stepProfileName

			case stepProfileActivate:
				if len(createdProfiles) == 0 {
					step = stepDone
					continue
				}

				// Build sorted profile names.
				profileNames := make([]string, 0, len(createdProfiles))
				for name := range createdProfiles {
					profileNames = append(profileNames, name)
				}
				sort.Strings(profileNames)

				var options []huh.Option[string]
				for _, name := range profileNames {
					options = append(options, huh.NewOption(capitalize(name), name))
				}
				options = append(options, huh.NewOption("None (base only)", ""))

				fmt.Println()
				err := huh.NewForm(
					huh.NewGroup(
						huh.NewSelect[string]().
							Title("Activate a profile on this machine?").
							Options(options...).
							Value(&activeProfile),
					),
				).Run()
				if err != nil {
					if errors.Is(err, huh.ErrUserAborted) {
						step = stepProfileName
						continue
					}
					return err
				}

				step = stepDone
			}
		}

		// Phase 3: Run init with selections.
		fmt.Println()
		result, err := commands.Init(commands.InitOptions{
			ClaudeDir:       claudeDir,
			SyncDir:         syncDir,
			RemoteURL:       initRemote,
			IncludeSettings: includeSettings,
			IncludeHooks:    includeHooks,
			IncludePlugins:  includePlugins,
			Profiles:        createdProfiles,
			ActiveProfile:   activeProfile,
		})
		if err != nil {
			return err
		}

		fmt.Println("✓ Created ~/.claude-sync/")
		fmt.Println("✓ Generated config.yaml from current Claude Code setup")
		fmt.Println("✓ Initialized git repository")

		if len(result.Upstream) > 0 {
			fmt.Printf("\n  Upstream:    %d plugin(s)\n", len(result.Upstream))
		}
		if len(result.AutoForked) > 0 {
			fmt.Printf("  Auto-forked: %d plugin(s) (non-portable marketplace)\n", len(result.AutoForked))
			for _, p := range result.AutoForked {
				fmt.Printf("    → %s\n", p)
			}
		}
		if len(result.Skipped) > 0 {
			fmt.Printf("  Skipped:     %d plugin(s) (local scope)\n", len(result.Skipped))
			for _, p := range result.Skipped {
				fmt.Printf("    → %s\n", p)
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

		if result.RemotePushed {
			fmt.Println()
			fmt.Println("✓ Pushed to remote")
			fmt.Println()
			fmt.Println("Next steps:")
			fmt.Println("  On another machine: claude-sync join", initRemote)
		} else {
			fmt.Println()
			fmt.Println("Next steps:")
			fmt.Println("  1. Review config: cat ~/.claude-sync/config.yaml")
			fmt.Println("  2. Push: claude-sync push -m \"Initial config\"")
		}
		return nil
	},
}

func init() {
	initCmd.Flags().StringVarP(&initRemote, "remote", "r", "", "Git remote URL to add as origin and push to")
	initCmd.Flags().BoolVar(&initSkipPlugins, "skip-plugins", false, "Skip plugin selection prompt (include all)")
	initCmd.Flags().BoolVar(&initSkipSettings, "skip-settings", false, "Don't include settings in sync config")
	initCmd.Flags().BoolVar(&initSkipHooks, "skip-hooks", false, "Don't include hooks in sync config")
	initCmd.Flags().BoolVar(&initSkipProfiles, "skip-profiles", false, "Skip profile creation prompt")
}
