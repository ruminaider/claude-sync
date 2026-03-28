package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/ruminaider/claude-sync/internal/commands"
	"github.com/ruminaider/claude-sync/internal/paths"
	"github.com/ruminaider/claude-sync/internal/plugins"
	"github.com/ruminaider/claude-sync/internal/profiles"
	"github.com/spf13/cobra"
)

var quietFlag bool
var autoFlag bool
var forceFlag bool

var pullCmd = &cobra.Command{
	Use:   "pull",
	Short: "Pull latest config and apply locally",
	RunE: func(cmd *cobra.Command, args []string) error {
		var result *commands.PullResult
		var err error

		claudeDir := paths.ClaudeDir()
		syncDir := paths.SyncDir()

		if autoFlag {
			projectDir := readCWDFromStdin()
			result, err = commands.PullWithOptions(commands.PullOptions{
				ClaudeDir:  claudeDir,
				SyncDir:    syncDir,
				Quiet:      true,
				Auto:       true,
				Force:      forceFlag,
				ProjectDir: projectDir,
			})
		} else {
			// Preview incoming changes before applying.
			if !quietFlag {
				preview, previewErr := commands.PullPreview(syncDir)
				if previewErr != nil {
					fmt.Fprintf(os.Stderr, "Warning: could not preview changes: %v\n", previewErr)
				} else if preview.NothingToChange {
					fmt.Println(commands.FormatPullPreview(preview))
					// Still run pull to apply local-only changes (settings, plugins, etc.)
				} else {
					fmt.Println(commands.FormatPullPreview(preview))
					fmt.Println()
					confirm, promptErr := confirmPrompt("Apply these changes?")
					if promptErr != nil || !confirm {
						fmt.Println("Pull cancelled.")
						return nil
					}
				}
			}

			if !quietFlag {
				fmt.Println("Pulling latest config...")
			}
			result, err = commands.PullWithOptions(commands.PullOptions{
				ClaudeDir: claudeDir,
				SyncDir:   syncDir,
				Quiet:     quietFlag,
				Force:     forceFlag,
				DuplicateResolver: func(dupes []plugins.Duplicate) error {
					for _, d := range dupes {
						forkSrc, mktSrc, isFork := isForkDuplicate(d)
						if isFork {
							disable, promptErr := promptDisableForkOriginal(forkSrc, mktSrc)
							if promptErr != nil {
								return promptErr
							}
							if disable {
								resolution := forkPreferenceResolution(d.Name, forkSrc, mktSrc)
								if applyErr := plugins.ApplyResolution(claudeDir, syncDir, resolution); applyErr != nil {
									return fmt.Errorf("resolving fork duplicate %s: %w", d.Name, applyErr)
								}
								if !quietFlag {
									fmt.Printf("Resolved: disabled %s\n", mktSrc)
								}
							}
						} else {
							resolution, promptErr := promptDuplicateResolution(d, syncDir)
							if promptErr != nil {
								return promptErr
							}
							if applyErr := plugins.ApplyResolution(claudeDir, syncDir, resolution); applyErr != nil {
								return fmt.Errorf("resolving duplicate %s: %w", d.Name, applyErr)
							}
							if !quietFlag {
								fmt.Printf("Resolved: keeping %s\n", resolution.KeepSource)
							}
						}
					}
					return nil
				},
				ReEvalResolver: func(signals []plugins.ReEvalSignal) error {
					for _, sig := range signals {
						action, promptErr := promptReEvaluation(sig)
						if promptErr != nil {
							return promptErr
						}
						switch action {
						case "switch":
							if switchErr := plugins.ApplyReEvalSwitch(claudeDir, syncDir, sig.PluginName); switchErr != nil {
								return switchErr
							}
							if !quietFlag {
								fmt.Printf("Switched %s to marketplace\n", sig.PluginName)
							}
						case "snooze":
							if snoozeErr := plugins.ApplySnooze(syncDir, sig.PluginName, 2); snoozeErr != nil {
								return snoozeErr
							}
							if !quietFlag {
								fmt.Printf("Snoozed %s for 2 days\n", sig.PluginName)
							}
						case "keep":
							if resetErr := plugins.ResetReEvalBaseline(syncDir, sig.PluginName); resetErr != nil {
								return resetErr
							}
						}
					}
					return nil
				},
			})
		}
		if err != nil {
			return err
		}

		if quietFlag || autoFlag {
			// In auto mode, surface duplicate plugins so the agent can help resolve.
			if autoFlag && len(result.DuplicatePlugins) > 0 {
				for _, d := range result.DuplicatePlugins {
					forkSrc, mktSrc, isFork := isForkDuplicate(d)
					if isFork {
						resolution := forkPreferenceResolution(d.Name, forkSrc, mktSrc)
						if err := plugins.ApplyResolution(claudeDir, syncDir, resolution); err != nil {
							fmt.Fprintf(os.Stderr, "Warning: could not auto-resolve fork duplicate %s: %v\n", d.Name, err)
						} else {
							fmt.Fprintf(os.Stderr, "Auto-resolved: disabled %s (fork is active)\n", mktSrc)
						}
					} else {
						fmt.Fprintf(os.Stderr, "WARNING: duplicate plugin %s from multiple sources.\n", d.Name)
						fmt.Fprintf(os.Stderr, "  %s\n", strings.Join(d.Sources, ", "))
						fmt.Fprintf(os.Stderr, "To fix: set the unwanted source to false in enabledPlugins in ~/.claude/settings.json, then run 'claude-sync push'.\n")
					}
				}
			}
			// In auto mode, still show pending high-risk warnings.
			if autoFlag && len(result.PendingHighRisk) > 0 {
				fmt.Fprintf(os.Stderr, "%d high-risk change(s) deferred. Run 'claude-sync approve' to apply:\n", len(result.PendingHighRisk))
				for _, c := range result.PendingHighRisk {
					fmt.Fprintf(os.Stderr, "  - %s\n", c.Description)
				}
			}
			return nil
		}

		printPullResult(result)

		// Offer project init when in an unmanaged directory with profiles available.
		if result.ProjectInitEligible {
			if err := promptProjectInit(claudeDir, syncDir, result.ProjectInitDir, result.AvailableProfiles); err != nil {
				// User cancelled or declined — not an error.
				return nil
			}
		}

		return nil
	},
}

// promptProjectInit offers to initialize a project directory with a per-project profile.
// projectDir is the detected project root (not necessarily CWD).
func promptProjectInit(claudeDir, syncDir, projectDir string, profileNames []string) error {
	var wantInit bool
	if err := huh.NewForm(
		huh.NewGroup(
			huh.NewConfirm().
				Title(fmt.Sprintf("Initialize %s with a project profile?", filepath.Base(projectDir))).
				Description("Different directories can use different profiles.").
				Affirmative("Yes").
				Negative("Not now").
				Value(&wantInit),
		),
	).Run(); err != nil || !wantInit {
		return fmt.Errorf("declined")
	}

	// Show profile picker.
	active, _ := profiles.ReadActiveProfile(syncDir)
	options := make([]huh.Option[string], 0, len(profileNames)+1)
	for _, name := range profileNames {
		label := capitalize(name)
		if name == active {
			label += " (global default)"
		}
		options = append(options, huh.NewOption(label, name))
	}
	options = append(options, huh.NewOption("No profile — use base only", ""))

	var profile string
	if err := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Which profile should this project use?").
				Options(options...).
				Value(&profile),
		),
	).Run(); err != nil {
		return err
	}

	// Run project init.
	result, err := commands.ProjectInit(commands.ProjectInitOptions{
		ProjectDir:    projectDir,
		SyncDir:       syncDir,
		Profile:       profile,
		ProjectedKeys: []string{"hooks", "permissions"},
		Yes:           true,
	})
	if err != nil {
		return err
	}

	fmt.Printf("\nProject initialized at %s\n", projectDir)
	if result.Profile != "" {
		fmt.Printf("  Profile: %s\n", result.Profile)
	}
	fmt.Printf("  Projected keys: %s\n", strings.Join(result.ProjectedKeys, ", "))

	// Apply project settings now.
	pullResult, pullErr := commands.PullWithOptions(commands.PullOptions{
		ClaudeDir:  claudeDir,
		SyncDir:    syncDir,
		Quiet:      true,
		ProjectDir: projectDir,
	})
	if pullErr == nil && pullResult.ProjectSettingsApplied {
		fmt.Println("  Project settings applied.")
	}

	return nil
}

func init() {
	pullCmd.Flags().BoolVarP(&quietFlag, "quiet", "q", false, "Suppress output")
	pullCmd.Flags().BoolVar(&autoFlag, "auto", false, "Auto mode: apply safe changes, defer high-risk to pending")
	pullCmd.Flags().BoolVarP(&forceFlag, "force", "f", false, "Overwrite locally modified managed files")
}
