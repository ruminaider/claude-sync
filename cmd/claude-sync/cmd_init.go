package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/ruminaider/claude-sync/internal/claudemd"
	"github.com/ruminaider/claude-sync/internal/commands"
	"github.com/ruminaider/claude-sync/internal/config"
	"github.com/ruminaider/claude-sync/internal/paths"
	"github.com/ruminaider/claude-sync/internal/profiles"
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

type initStep int

const (
	stepConfigStyle         initStep = iota // ask simple vs profiles
	stepPluginStrategy
	stepPluginPicker
	stepClaudeMDStrategy    // NEW: 3-option for CLAUDE.md sections
	stepClaudeMDPicker      // NEW: per-fragment picker
	stepSettingsStrategy    // RENAMED from stepSettings: 3-option for settings
	stepSettingsPicker      // NEW: per-key picker
	stepPermissionsStrategy // NEW: 3-option for permissions
	stepPermissionsPicker   // NEW: per-rule picker
	stepMCPStrategy         // NEW: 3-option for MCP servers
	stepMCPPicker           // NEW: per-server picker
	stepKeybindings         // NEW: yes/no for keybindings
	stepHookStrategy
	stepHookPicker
	stepProfileName
	stepProfilePlugins
	stepProfileClaudeMD     // NEW
	stepProfileSettings
	stepProfilePermissions  // NEW
	stepProfileMCP          // NEW
	stepProfileKeybindings  // NEW
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
		if len(scan.Permissions.Allow) > 0 || len(scan.Permissions.Deny) > 0 {
			fmt.Printf("Found permissions: %d allow, %d deny\n", len(scan.Permissions.Allow), len(scan.Permissions.Deny))
		}
		if scan.ClaudeMDContent != "" {
			sections := len(strings.Split(scan.ClaudeMDContent, "\n## "))
			fmt.Printf("Found CLAUDE.md (%d section(s))\n", sections)
		}
		if len(scan.MCP) > 0 {
			mcpNames := make([]string, 0, len(scan.MCP))
			for k := range scan.MCP {
				mcpNames = append(mcpNames, k)
			}
			sort.Strings(mcpNames)
			fmt.Printf("Found MCP servers: %s\n", strings.Join(mcpNames, ", "))
		}
		if len(scan.Keybindings) > 0 {
			fmt.Printf("Found keybindings: %d\n", len(scan.Keybindings))
		}

		// Phase 2: Interactive prompts with go-back navigation.
		var includePlugins []string                 // nil = all
		includeSettings := true
		var settingsFilter []string                    // nil = all (when includeSettings is true)
		var includeHooks map[string]json.RawMessage    // nil = all
		importClaudeMD := scan.ClaudeMDContent != ""   // default: import if available
		var includeClaudeMDFragments []string           // nil = all, []string{} = none
		includePermissions := scan.Permissions          // default: include all
		includeMCP := scan.MCP                          // nil/empty = none, map = selected; default: all
		includeKeybindingsFlag := true                  // default: include

		// Profile-related variables.
		var useProfiles bool
		var createdProfiles map[string]profiles.Profile // name -> profile
		var activeProfile string
		var currentProfileName string
		var currentProfile profiles.Profile

		// Store all plugin keys for the profile picker.
		allPluginKeys := scan.PluginKeys

		// Determine the starting step based on flags and data availability.
		hasPlugins := len(scan.PluginKeys) > 0 && !initSkipPlugins
		hasClaudeMD := scan.ClaudeMDContent != "" && !initSkipClaudeMD
		hasSettings := len(scan.Settings) > 0 && !initSkipSettings
		hasPermissions := (len(scan.Permissions.Allow) > 0 || len(scan.Permissions.Deny) > 0) && !initSkipPermissions
		hasMCP := len(scan.MCP) > 0 && !initSkipMCP
		hasKeybindings := len(scan.Keybindings) > 0 && !initSkipKeybindings
		hasHooks := len(scan.Hooks) > 0 && !initSkipHooks

		if initSkipSettings {
			includeSettings = false
		}
		if initSkipHooks {
			includeHooks = map[string]json.RawMessage{} // empty = none
		}
		if initSkipClaudeMD {
			importClaudeMD = false
		}
		if initSkipPermissions {
			includePermissions = config.Permissions{}
		}
		if initSkipMCP {
			includeMCP = nil
		}
		if initSkipKeybindings {
			includeKeybindingsFlag = false
		}

		// nextBaseStep returns the next step in the base config chain after the given step.
		nextBaseStep := func(after initStep) initStep {
			order := []struct {
				step    initStep
				enabled bool
			}{
				{stepClaudeMDStrategy, hasClaudeMD},
				{stepSettingsStrategy, hasSettings},
				{stepPermissionsStrategy, hasPermissions},
				{stepMCPStrategy, hasMCP},
				{stepKeybindings, hasKeybindings},
				{stepHookStrategy, hasHooks},
			}
			found := false
			for _, entry := range order {
				if entry.step == after {
					found = true
					continue
				}
				if found && entry.enabled {
					return entry.step
				}
			}
			return stepDone
		}

		// prevBaseStep returns the previous step in the base config chain before the given step.
		prevBaseStep := func(before initStep) initStep {
			order := []struct {
				step    initStep
				enabled bool
			}{
				{stepPluginStrategy, hasPlugins},
				{stepClaudeMDStrategy, hasClaudeMD},
				{stepSettingsStrategy, hasSettings},
				{stepPermissionsStrategy, hasPermissions},
				{stepMCPStrategy, hasMCP},
				{stepKeybindings, hasKeybindings},
				{stepHookStrategy, hasHooks},
			}
			prev := initStep(-1)
			for _, entry := range order {
				if entry.step == before {
					break
				}
				if entry.enabled {
					prev = entry.step
				}
			}
			return prev
		}

		step := stepDone
		if hasHooks {
			step = stepHookStrategy
		}
		if hasKeybindings {
			step = stepKeybindings
		}
		if hasMCP {
			step = stepMCPStrategy
		}
		if hasPermissions {
			step = stepPermissionsStrategy
		}
		if hasSettings {
			step = stepSettingsStrategy
		}
		if hasClaudeMD {
			step = stepClaudeMDStrategy
		}
		if hasPlugins {
			step = stepPluginStrategy
		}
		if !initSkipProfiles && hasPlugins {
			step = stepConfigStyle
		}
		firstStep := step

		for step != stepDone {
			switch step {
			case stepConfigStyle:
				var choice string
				fmt.Println()
				err := huh.NewForm(
					huh.NewGroup(
						huh.NewSelect[string]().
							Title("Configuration style:").
							Options(
								huh.NewOption("Simple (single config)", "simple"),
								huh.NewOption("With profiles (e.g., work, personal)", "profiles"),
							).
							Value(&choice),
					),
				).Run()
				if err != nil {
					if errors.Is(err, huh.ErrUserAborted) {
						return err
					}
					return err
				}
				useProfiles = (choice == "profiles")
				step = stepPluginStrategy

			case stepPluginStrategy:
				title := fmt.Sprintf("Include all %d plugins in sync?", len(scan.PluginKeys))
				optAll := "Share all (Recommended)"
				optSome := "Choose which to share"
				optNone := "Don't share any plugins"
				if useProfiles {
					title = fmt.Sprintf("Base plugins (shared by all profiles) — %d found:", len(scan.PluginKeys))
					optAll = "Include all (Recommended)"
					optSome = "Choose which to include"
					optNone = "Don't include any plugins"
				}

				var choice string
				fmt.Println()
				err := huh.NewForm(
					huh.NewGroup(
						huh.NewSelect[string]().
							Title(title).
							Options(
								huh.NewOption(optAll, "all"),
								huh.NewOption(optSome, "some"),
								huh.NewOption(optNone, "none"),
							).
							Value(&choice),
					),
				).Run()
				if err != nil {
					if errors.Is(err, huh.ErrUserAborted) {
						if firstStep == stepConfigStyle {
							step = stepConfigStyle
							continue
						}
						return err
					}
					return err
				}

				switch choice {
				case "all":
					includePlugins = nil // nil = all
					step = nextBaseStep(stepPluginStrategy)
				case "none":
					includePlugins = []string{} // empty = none
					step = nextBaseStep(stepPluginStrategy)
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

				pickerTitle := "Select plugins to share:"
				if useProfiles {
					pickerTitle = "Select base plugins:"
				}

				selected, err := runPickerWithSections(pickerTitle, sections)
				if err != nil {
					if errors.Is(err, huh.ErrUserAborted) {
						step = stepPluginStrategy
						continue
					}
					return err
				}
				includePlugins = selected
				step = nextBaseStep(stepPluginStrategy)

			case stepClaudeMDStrategy:
				// Build section display names.
				sectionNames := make([]string, 0, len(scan.ClaudeMDSections))
				for _, sec := range scan.ClaudeMDSections {
					name := sec.Header
					if name == "" {
						name = "(preamble)"
					}
					sectionNames = append(sectionNames, name)
				}
				sectionSummary := strings.Join(sectionNames, ", ")
				n := len(scan.ClaudeMDSections)

				title := fmt.Sprintf("Include CLAUDE.md sections? (%d section(s): %s)", n, sectionSummary)
				if useProfiles {
					title = fmt.Sprintf("Base CLAUDE.md sections — %d section(s): %s", n, sectionSummary)
				}

				var choice string
				fmt.Println()
				err := huh.NewForm(
					huh.NewGroup(
						huh.NewSelect[string]().
							Title(title).
							Options(
								huh.NewOption(fmt.Sprintf("Include all %d section(s)", n), "all"),
								huh.NewOption("Choose which to include", "some"),
								huh.NewOption("Don't include", "none"),
							).
							Value(&choice),
					),
				).Run()
				if err != nil {
					if errors.Is(err, huh.ErrUserAborted) {
						prev := prevBaseStep(stepClaudeMDStrategy)
						if prev >= 0 {
							step = prev
							continue
						}
						if firstStep == stepConfigStyle {
							step = stepConfigStyle
							continue
						}
						return err
					}
					return err
				}

				switch choice {
				case "all":
					importClaudeMD = true
					includeClaudeMDFragments = nil // nil = all
					step = nextBaseStep(stepClaudeMDStrategy)
				case "none":
					importClaudeMD = false
					step = nextBaseStep(stepClaudeMDStrategy)
				case "some":
					step = stepClaudeMDPicker
				}

			case stepClaudeMDPicker:
				// Build display items from sections.
				displayItems := make([]string, 0, len(scan.ClaudeMDSections))
				displayToFragment := make(map[string]string)
				for _, sec := range scan.ClaudeMDSections {
					display := sec.Header
					if display == "" {
						display = "(preamble)"
					}
					displayItems = append(displayItems, display)
					displayToFragment[display] = claudemd.HeaderToFragmentName(sec.Header)
				}

				pickerTitle := "Select CLAUDE.md sections to include:"
				if useProfiles {
					pickerTitle = "Select base CLAUDE.md sections:"
				}

				selected, err := runPicker(pickerTitle, displayItems)
				if err != nil {
					if errors.Is(err, huh.ErrUserAborted) {
						step = stepClaudeMDStrategy
						continue
					}
					return err
				}
				importClaudeMD = true
				includeClaudeMDFragments = make([]string, 0, len(selected))
				for _, display := range selected {
					includeClaudeMDFragments = append(includeClaudeMDFragments, displayToFragment[display])
				}
				step = nextBaseStep(stepClaudeMDStrategy)

			case stepSettingsStrategy:
				keys := make([]string, 0, len(scan.Settings))
				for k := range scan.Settings {
					keys = append(keys, k)
				}
				sort.Strings(keys)
				n := len(keys)

				title := fmt.Sprintf("Include settings? (%s)", strings.Join(keys, ", "))
				if useProfiles {
					title = fmt.Sprintf("Base settings — %s", strings.Join(keys, ", "))
				}

				var choice string
				fmt.Println()
				err := huh.NewForm(
					huh.NewGroup(
						huh.NewSelect[string]().
							Title(title).
							Options(
								huh.NewOption(fmt.Sprintf("Include all %d setting(s)", n), "all"),
								huh.NewOption("Choose which to include", "some"),
								huh.NewOption("Don't include", "none"),
							).
							Value(&choice),
					),
				).Run()
				if err != nil {
					if errors.Is(err, huh.ErrUserAborted) {
						prev := prevBaseStep(stepSettingsStrategy)
						if prev >= 0 {
							step = prev
							continue
						}
						if firstStep == stepConfigStyle {
							step = stepConfigStyle
							continue
						}
						return err
					}
					return err
				}

				switch choice {
				case "all":
					includeSettings = true
					settingsFilter = nil // nil = all
					step = nextBaseStep(stepSettingsStrategy)
				case "none":
					includeSettings = false
					step = nextBaseStep(stepSettingsStrategy)
				case "some":
					step = stepSettingsPicker
				}

			case stepSettingsPicker:
				// Build display strings: "key: value"
				keys := make([]string, 0, len(scan.Settings))
				for k := range scan.Settings {
					keys = append(keys, k)
				}
				sort.Strings(keys)

				displayItems := make([]string, 0, len(keys))
				displayToKey := make(map[string]string)
				for _, k := range keys {
					display := fmt.Sprintf("%s: %v", k, scan.Settings[k])
					displayItems = append(displayItems, display)
					displayToKey[display] = k
				}

				pickerTitle := "Select settings to include:"
				if useProfiles {
					pickerTitle = "Select base settings:"
				}

				selected, err := runPicker(pickerTitle, displayItems)
				if err != nil {
					if errors.Is(err, huh.ErrUserAborted) {
						step = stepSettingsStrategy
						continue
					}
					return err
				}
				includeSettings = true
				settingsFilter = make([]string, 0, len(selected))
				for _, display := range selected {
					settingsFilter = append(settingsFilter, displayToKey[display])
				}
				step = nextBaseStep(stepSettingsStrategy)

			case stepPermissionsStrategy:
				allowCount := len(scan.Permissions.Allow)
				denyCount := len(scan.Permissions.Deny)

				title := fmt.Sprintf("Include permissions? (%d allow, %d deny rules)", allowCount, denyCount)
				if useProfiles {
					title = fmt.Sprintf("Base permissions — %d allow, %d deny rules", allowCount, denyCount)
				}

				var choice string
				fmt.Println()
				err := huh.NewForm(
					huh.NewGroup(
						huh.NewSelect[string]().
							Title(title).
							Options(
								huh.NewOption("Include all", "all"),
								huh.NewOption("Choose which to include", "some"),
								huh.NewOption("Don't include", "none"),
							).
							Value(&choice),
					),
				).Run()
				if err != nil {
					if errors.Is(err, huh.ErrUserAborted) {
						prev := prevBaseStep(stepPermissionsStrategy)
						if prev >= 0 {
							step = prev
							continue
						}
						if firstStep == stepConfigStyle {
							step = stepConfigStyle
							continue
						}
						return err
					}
					return err
				}

				switch choice {
				case "all":
					includePermissions = scan.Permissions
					step = nextBaseStep(stepPermissionsStrategy)
				case "none":
					includePermissions = config.Permissions{}
					step = nextBaseStep(stepPermissionsStrategy)
				case "some":
					step = stepPermissionsPicker
				}

			case stepPermissionsPicker:
				// Build two sections: Allow and Deny.
				var sections []pickerSection
				allowSet := make(map[string]bool)
				if len(scan.Permissions.Allow) > 0 {
					sections = append(sections, pickerSection{
						Header: fmt.Sprintf("Allow (%d)", len(scan.Permissions.Allow)),
						Items:  scan.Permissions.Allow,
					})
					for _, r := range scan.Permissions.Allow {
						allowSet[r] = true
					}
				}
				if len(scan.Permissions.Deny) > 0 {
					sections = append(sections, pickerSection{
						Header: fmt.Sprintf("Deny (%d)", len(scan.Permissions.Deny)),
						Items:  scan.Permissions.Deny,
					})
				}

				pickerTitle := "Select permission rules to include:"
				if useProfiles {
					pickerTitle = "Select base permission rules:"
				}

				selected, err := runPickerWithSections(pickerTitle, sections)
				if err != nil {
					if errors.Is(err, huh.ErrUserAborted) {
						step = stepPermissionsStrategy
						continue
					}
					return err
				}
				// Separate selected items back into allow/deny.
				var filteredAllow, filteredDeny []string
				for _, item := range selected {
					if allowSet[item] {
						filteredAllow = append(filteredAllow, item)
					} else {
						filteredDeny = append(filteredDeny, item)
					}
				}
				includePermissions = config.Permissions{Allow: filteredAllow, Deny: filteredDeny}
				step = nextBaseStep(stepPermissionsStrategy)

			case stepMCPStrategy:
				mcpNames := make([]string, 0, len(scan.MCP))
				for k := range scan.MCP {
					mcpNames = append(mcpNames, k)
				}
				sort.Strings(mcpNames)
				n := len(mcpNames)

				title := fmt.Sprintf("Include MCP servers? (%s)", strings.Join(mcpNames, ", "))
				if useProfiles {
					title = fmt.Sprintf("Base MCP servers — %s", strings.Join(mcpNames, ", "))
				}

				var choice string
				fmt.Println()
				err := huh.NewForm(
					huh.NewGroup(
						huh.NewSelect[string]().
							Title(title).
							Options(
								huh.NewOption(fmt.Sprintf("Include all %d server(s)", n), "all"),
								huh.NewOption("Choose which to include", "some"),
								huh.NewOption("Don't include", "none"),
							).
							Value(&choice),
					),
				).Run()
				if err != nil {
					if errors.Is(err, huh.ErrUserAborted) {
						prev := prevBaseStep(stepMCPStrategy)
						if prev >= 0 {
							step = prev
							continue
						}
						if firstStep == stepConfigStyle {
							step = stepConfigStyle
							continue
						}
						return err
					}
					return err
				}

				switch choice {
				case "all":
					includeMCP = scan.MCP
					step = nextBaseStep(stepMCPStrategy)
				case "none":
					includeMCP = nil
					step = nextBaseStep(stepMCPStrategy)
				case "some":
					step = stepMCPPicker
				}

			case stepMCPPicker:
				mcpNames := make([]string, 0, len(scan.MCP))
				for k := range scan.MCP {
					mcpNames = append(mcpNames, k)
				}
				sort.Strings(mcpNames)

				pickerTitle := "Select MCP servers to include:"
				if useProfiles {
					pickerTitle = "Select base MCP servers:"
				}

				selected, err := runPicker(pickerTitle, mcpNames)
				if err != nil {
					if errors.Is(err, huh.ErrUserAborted) {
						step = stepMCPStrategy
						continue
					}
					return err
				}
				includeMCP = make(map[string]json.RawMessage)
				for _, name := range selected {
					includeMCP[name] = scan.MCP[name]
				}
				step = nextBaseStep(stepMCPStrategy)

			case stepKeybindings:
				n := len(scan.Keybindings)
				title := fmt.Sprintf("Include keybindings? (%d binding(s))", n)
				if useProfiles {
					title = fmt.Sprintf("Include keybindings in base config? (%d binding(s))", n)
				}

				var confirm bool
				fmt.Println()
				err := huh.NewForm(
					huh.NewGroup(
						huh.NewConfirm().
							Title(title).
							Affirmative("Yes").
							Negative("No").
							Value(&confirm),
					),
				).Run()
				if err != nil {
					if errors.Is(err, huh.ErrUserAborted) {
						prev := prevBaseStep(stepKeybindings)
						if prev >= 0 {
							step = prev
							continue
						}
						if firstStep == stepConfigStyle {
							step = stepConfigStyle
							continue
						}
						return err
					}
					return err
				}
				includeKeybindingsFlag = confirm
				step = nextBaseStep(stepKeybindings)

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

				hookTitle := fmt.Sprintf("Sync hooks? (Found: %s)", strings.Join(hookSummary, ", "))
				hookOptAll := "Share all"
				hookOptSome := "Choose which to share"
				hookOptNone := "Don't share hooks"
				if useProfiles {
					hookTitle = fmt.Sprintf("Base hooks (shared by all profiles) — Found: %s", strings.Join(hookSummary, ", "))
					hookOptAll = "Include all"
					hookOptSome = "Choose which to include"
					hookOptNone = "Don't include hooks"
				}

				var choice string
				fmt.Println()
				err := huh.NewForm(
					huh.NewGroup(
						huh.NewSelect[string]().
							Title(hookTitle).
							Options(
								huh.NewOption(hookOptAll, "all"),
								huh.NewOption(hookOptSome, "some"),
								huh.NewOption(hookOptNone, "none"),
							).
							Value(&choice),
					),
				).Run()
				if err != nil {
					if errors.Is(err, huh.ErrUserAborted) {
						prev := prevBaseStep(stepHookStrategy)
						if prev >= 0 {
							step = prev
							continue
						}
						if firstStep == stepConfigStyle {
							step = stepConfigStyle
							continue
						}
						return err
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

				hookPickerTitle := "Select hooks to share:"
				if useProfiles {
					hookPickerTitle = "Select base hooks:"
				}

				selected, err := runPicker(hookPickerTitle, displayItems)
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

		// Profile creation loop — only enter if user chose "With profiles".
		if useProfiles {
			// Show a summary of base config before entering profile creation.
			fmt.Println()
			var summaryParts []string
			summaryParts = append(summaryParts, fmt.Sprintf("%d plugin(s)", len(basePluginKeys)))
			if importClaudeMD {
				fragCount := len(scan.ClaudeMDSections)
				if includeClaudeMDFragments != nil {
					fragCount = len(includeClaudeMDFragments)
				}
				if fragCount > 0 {
					summaryParts = append(summaryParts, fmt.Sprintf("%d CLAUDE.md fragment(s)", fragCount))
				}
			}
			if includeSettings {
				settingCount := len(scan.Settings)
				if settingsFilter != nil {
					settingCount = len(settingsFilter)
				}
				if settingCount > 0 {
					summaryParts = append(summaryParts, fmt.Sprintf("%d setting(s)", settingCount))
				}
			}
			if len(includePermissions.Allow) > 0 || len(includePermissions.Deny) > 0 {
				summaryParts = append(summaryParts, fmt.Sprintf("%d allow + %d deny permissions", len(includePermissions.Allow), len(includePermissions.Deny)))
			}
			if len(includeMCP) > 0 {
				summaryParts = append(summaryParts, fmt.Sprintf("%d MCP server(s)", len(includeMCP)))
			}
			if includeKeybindingsFlag && len(scan.Keybindings) > 0 {
				summaryParts = append(summaryParts, fmt.Sprintf("%d keybinding(s)", len(scan.Keybindings)))
			}
			if includeHooks == nil || len(includeHooks) > 0 {
				hookCount := len(scan.Hooks)
				if includeHooks != nil {
					hookCount = len(includeHooks)
				}
				if hookCount > 0 {
					summaryParts = append(summaryParts, fmt.Sprintf("%d hook(s)", hookCount))
				}
			}
			fmt.Printf("Base configured: %s\n", strings.Join(summaryParts, ", "))

			createdProfiles = make(map[string]profiles.Profile)
			step = stepProfileName
		}

		for step != stepDone {
			switch step {
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
						// Esc from profile name — skip profiles entirely.
						step = stepDone
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
				step = stepProfileClaudeMD

			case stepProfileClaudeMD:
				// Check if base has CLAUDE.md. If not, skip to settings.
				if !importClaudeMD || len(includeClaudeMDFragments) == 0 && includeClaudeMDFragments != nil {
					step = stepProfileSettings
					continue
				}

				// Determine base fragments.
				baseFragments := includeClaudeMDFragments
				if baseFragments == nil {
					// nil = all; build from scan sections
					baseFragments = make([]string, 0, len(scan.ClaudeMDSections))
					for _, sec := range scan.ClaudeMDSections {
						baseFragments = append(baseFragments, claudemd.HeaderToFragmentName(sec.Header))
					}
				}

				// Build display items and sections.
				displayItems := make([]string, 0, len(scan.ClaudeMDSections))
				displayToFragment := make(map[string]string)
				for _, sec := range scan.ClaudeMDSections {
					display := sec.Header
					if display == "" {
						display = "(preamble)"
					}
					displayItems = append(displayItems, display)
					displayToFragment[display] = claudemd.HeaderToFragmentName(sec.Header)
				}

				// Pre-select base fragments.
				baseFragSet := make(map[string]bool, len(baseFragments))
				for _, f := range baseFragments {
					baseFragSet[f] = true
				}
				preSelected := make(map[string]bool, len(displayItems))
				for _, display := range displayItems {
					if baseFragSet[displayToFragment[display]] {
						preSelected[display] = true
					}
				}

				sections := []pickerSection{{
					Header: fmt.Sprintf("CLAUDE.md sections (%d)", len(displayItems)),
					Items:  displayItems,
				}}

				selected, err := runPickerWithPreSelected(
					fmt.Sprintf("Select CLAUDE.md sections for %q:", currentProfileName),
					sections,
					preSelected,
				)
				if err != nil {
					if errors.Is(err, huh.ErrUserAborted) {
						step = stepProfilePlugins
						continue
					}
					return err
				}

				// Compute diff: adds and removes relative to base fragments.
				selectedFrags := make(map[string]bool, len(selected))
				for _, display := range selected {
					selectedFrags[displayToFragment[display]] = true
				}
				var fragAdds, fragRemoves []string
				for _, display := range selected {
					frag := displayToFragment[display]
					if !baseFragSet[frag] {
						fragAdds = append(fragAdds, frag)
					}
				}
				for _, f := range baseFragments {
					if !selectedFrags[f] {
						fragRemoves = append(fragRemoves, f)
					}
				}
				if len(fragAdds) > 0 || len(fragRemoves) > 0 {
					currentProfile.ClaudeMD = profiles.ProfileClaudeMD{Add: fragAdds, Remove: fragRemoves}
				}

				step = stepProfileSettings

			case stepProfileSettings:
				// Check if base has settings. If not, skip to permissions.
				if !includeSettings || len(scan.Settings) == 0 {
					step = stepProfilePermissions
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

				step = stepProfilePermissions

			case stepProfilePermissions:
				// Check if base has permissions. If not, skip to MCP.
				if len(includePermissions.Allow) == 0 && len(includePermissions.Deny) == 0 {
					step = stepProfileMCP
					continue
				}

				var addPerms bool
				fmt.Println()
				err := huh.NewForm(
					huh.NewGroup(
						huh.NewConfirm().
							Title(fmt.Sprintf("Add extra permission rules for %q?", currentProfileName)).
							Affirmative("Yes").
							Negative("No").
							Value(&addPerms),
					),
				).Run()
				if err != nil {
					if errors.Is(err, huh.ErrUserAborted) {
						step = stepProfileSettings
						continue
					}
					return err
				}

				if addPerms {
					var allowInput string
					var denyInput string
					fmt.Println()
					err := huh.NewForm(
						huh.NewGroup(
							huh.NewInput().
								Title("Additional allow rules (comma-separated, or empty):").
								Value(&allowInput),
							huh.NewInput().
								Title("Additional deny rules (comma-separated, or empty):").
								Value(&denyInput),
						),
					).Run()
					if err != nil {
						if errors.Is(err, huh.ErrUserAborted) {
							continue
						}
						return err
					}

					var addAllow, addDeny []string
					for _, s := range strings.Split(allowInput, ",") {
						s = strings.TrimSpace(s)
						if s != "" {
							addAllow = append(addAllow, s)
						}
					}
					for _, s := range strings.Split(denyInput, ",") {
						s = strings.TrimSpace(s)
						if s != "" {
							addDeny = append(addDeny, s)
						}
					}
					if len(addAllow) > 0 || len(addDeny) > 0 {
						currentProfile.Permissions = profiles.ProfilePermissions{AddAllow: addAllow, AddDeny: addDeny}
					}
				}

				step = stepProfileMCP

			case stepProfileMCP:
				// Check if base has MCP servers. If not, skip to keybindings.
				if len(includeMCP) == 0 {
					step = stepProfileKeybindings
					continue
				}

				// Build sections: base MCP servers and all available.
				baseMCPNames := make([]string, 0, len(includeMCP))
				for k := range includeMCP {
					baseMCPNames = append(baseMCPNames, k)
				}
				sort.Strings(baseMCPNames)

				// Build all MCP names (base + any from scan not in base).
				allMCPNames := make([]string, 0, len(scan.MCP))
				for k := range scan.MCP {
					allMCPNames = append(allMCPNames, k)
				}
				sort.Strings(allMCPNames)

				baseMCPSet := make(map[string]bool, len(baseMCPNames))
				for _, n := range baseMCPNames {
					baseMCPSet[n] = true
				}

				var baseItems, nonBaseItems []string
				for _, n := range allMCPNames {
					if baseMCPSet[n] {
						baseItems = append(baseItems, n)
					} else {
						nonBaseItems = append(nonBaseItems, n)
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

				preSelected := make(map[string]bool, len(baseItems))
				for _, n := range baseItems {
					preSelected[n] = true
				}

				selected, err := runPickerWithPreSelected(
					fmt.Sprintf("Select MCP servers for %q:", currentProfileName),
					sections,
					preSelected,
				)
				if err != nil {
					if errors.Is(err, huh.ErrUserAborted) {
						step = stepProfilePermissions
						continue
					}
					return err
				}

				// Compute diff.
				selectedSet := make(map[string]bool, len(selected))
				for _, s := range selected {
					selectedSet[s] = true
				}
				mcpAdd := make(map[string]json.RawMessage)
				for _, s := range selected {
					if !baseMCPSet[s] {
						mcpAdd[s] = scan.MCP[s]
					}
				}
				var mcpRemoves []string
				for _, b := range baseMCPNames {
					if !selectedSet[b] {
						mcpRemoves = append(mcpRemoves, b)
					}
				}
				if len(mcpAdd) > 0 || len(mcpRemoves) > 0 {
					currentProfile.MCP = profiles.ProfileMCP{Add: mcpAdd, Remove: mcpRemoves}
				}

				step = stepProfileKeybindings

			case stepProfileKeybindings:
				// Check if base has keybindings. If not, skip to hooks.
				if !includeKeybindingsFlag || len(scan.Keybindings) == 0 {
					step = stepProfileHooks
					continue
				}

				var overrideKB bool
				fmt.Println()
				err := huh.NewForm(
					huh.NewGroup(
						huh.NewConfirm().
							Title(fmt.Sprintf("Override keybindings for %q?", currentProfileName)).
							Affirmative("Yes").
							Negative("No").
							Value(&overrideKB),
					),
				).Run()
				if err != nil {
					if errors.Is(err, huh.ErrUserAborted) {
						step = stepProfileMCP
						continue
					}
					return err
				}

				if overrideKB {
					// Allow comma-separated key=value overrides.
					var overrideInput string
					err := huh.NewForm(
						huh.NewGroup(
							huh.NewInput().
								Title("Keybinding overrides (key=value, comma-separated):").
								Value(&overrideInput),
						),
					).Run()
					if err != nil {
						if errors.Is(err, huh.ErrUserAborted) {
							continue
						}
						return err
					}
					overrides := make(map[string]any)
					for _, pair := range strings.Split(overrideInput, ",") {
						pair = strings.TrimSpace(pair)
						if pair == "" {
							continue
						}
						parts := strings.SplitN(pair, "=", 2)
						if len(parts) == 2 {
							overrides[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
						}
					}
					if len(overrides) > 0 {
						currentProfile.Keybindings = profiles.ProfileKeybindings{Override: overrides}
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
						step = stepProfileKeybindings
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

		// Resolve keybindings: only include if user opted in.
		var finalKeybindings map[string]any
		if includeKeybindingsFlag {
			finalKeybindings = scan.Keybindings
		}

		result, err := commands.Init(commands.InitOptions{
			ClaudeDir:         claudeDir,
			SyncDir:           syncDir,
			RemoteURL:         initRemote,
			IncludeSettings:   includeSettings,
			SettingsFilter:    settingsFilter,
			IncludeHooks:      includeHooks,
			IncludePlugins:    includePlugins,
			Profiles:          createdProfiles,
			ActiveProfile:     activeProfile,
			Permissions:       includePermissions,
			ImportClaudeMD:    importClaudeMD,
			ClaudeMDFragments: includeClaudeMDFragments,
			MCP:               includeMCP,
			Keybindings:       finalKeybindings,
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
	initCmd.Flags().BoolVar(&initSkipClaudeMD, "skip-claude-md", false, "Don't include CLAUDE.md in sync config")
	initCmd.Flags().BoolVar(&initSkipPermissions, "skip-permissions", false, "Don't include permissions in sync config")
	initCmd.Flags().BoolVar(&initSkipMCP, "skip-mcp", false, "Don't include MCP servers in sync config")
	initCmd.Flags().BoolVar(&initSkipKeybindings, "skip-keybindings", false, "Don't include keybindings in sync config")
}
