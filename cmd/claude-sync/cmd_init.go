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
	"github.com/spf13/cobra"
)

var (
	initRemote       string
	initSkipSettings bool
	initSkipHooks    bool
)

type initStep int

const (
	stepSettings     initStep = iota
	stepHookStrategy initStep = iota
	stepHookPicker   initStep = iota
	stepDone         initStep = iota
)

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
		includeSettings := true
		var includeHooks map[string]json.RawMessage // nil = all

		// Determine the starting step based on flags and data availability.
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
		firstStep := step

		for step != stepDone {
			switch step {
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
					if errors.Is(err, huh.ErrUserAborted) && step == firstStep {
						return err
					}
					if errors.Is(err, huh.ErrUserAborted) {
						// Can't go back further from settings, abort.
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
						step = stepSettings
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

		// Phase 3: Run init with selections.
		fmt.Println()
		result, err := commands.Init(commands.InitOptions{
			ClaudeDir:       claudeDir,
			SyncDir:         syncDir,
			RemoteURL:       initRemote,
			IncludeSettings: includeSettings,
			IncludeHooks:    includeHooks,
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
		if len(result.IncludedSettings) > 0 {
			fmt.Printf("  Settings:    %s\n", strings.Join(result.IncludedSettings, ", "))
		}
		if len(result.IncludedHooks) > 0 {
			fmt.Printf("  Hooks:       %s\n", strings.Join(result.IncludedHooks, ", "))
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
	initCmd.Flags().BoolVar(&initSkipSettings, "skip-settings", false, "Don't include settings in sync config")
	initCmd.Flags().BoolVar(&initSkipHooks, "skip-hooks", false, "Don't include hooks in sync config")
}
