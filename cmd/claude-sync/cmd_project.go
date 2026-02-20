package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/ruminaider/claude-sync/internal/commands"
	"github.com/ruminaider/claude-sync/internal/paths"
	"github.com/ruminaider/claude-sync/internal/profiles"
	"github.com/spf13/cobra"
)

var projectCmd = &cobra.Command{
	Use:   "project",
	Short: "Manage per-project claude-sync settings",
	Long:  "Commands for initializing, listing, and removing per-project settings.local.json management.",
}

var (
	projectInitProfile string
	projectInitKeys    string
	projectInitYes     bool
	projectInitDecline bool
)

var projectInitCmd = &cobra.Command{
	Use:   "init [path]",
	Short: "Initialize a project's settings.local.json from a profile",
	Long: `Initialize claude-sync management for a project directory.

This command:
  1. Detects existing settings.local.json and imports project-specific overrides
  2. Creates .claude/.claude-sync.yaml with profile reference and overrides
  3. Regenerates settings.local.json with managed keys from the resolved profile

Examples:
  claude-sync project init                    # current directory
  claude-sync project init ~/Work/my-project  # specific path
  claude-sync project init --profile work --keys hooks,permissions --yes`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		projectDir := "."
		if len(args) > 0 {
			projectDir = args[0]
		}
		projectDir, _ = filepath.Abs(projectDir)
		syncDir := paths.SyncDir()

		// Handle decline
		if projectInitDecline {
			return declineProject(projectDir)
		}

		// Profile selection
		profile := projectInitProfile
		if profile == "" && !projectInitYes {
			available, _ := profiles.ListProfiles(syncDir)
			active, _ := profiles.ReadActiveProfile(syncDir)
			if len(available) > 0 {
				selected, err := promptProjectProfile(available, active)
				if err != nil {
					return fmt.Errorf("cancelled")
				}
				profile = selected
			} else {
				profile = active
			}
		}

		// Key selection
		keys := []string{"hooks", "permissions"}
		if projectInitKeys != "" {
			keys = strings.Split(projectInitKeys, ",")
		}

		result, err := commands.ProjectInit(commands.ProjectInitOptions{
			ProjectDir:    projectDir,
			SyncDir:       syncDir,
			Profile:       profile,
			ProjectedKeys: keys,
			Yes:           projectInitYes,
		})
		if err != nil {
			return err
		}

		fmt.Printf("Project initialized at %s\n", projectDir)
		if result.Profile != "" {
			fmt.Printf("  Profile: %s\n", result.Profile)
		}
		fmt.Printf("  Projected keys: %s\n", strings.Join(result.ProjectedKeys, ", "))
		if result.ImportedPermissions > 0 {
			fmt.Printf("  Imported %d project-specific permission(s)\n", result.ImportedPermissions)
		}
		if result.ImportedHooks > 0 {
			fmt.Printf("  Imported %d project-specific hook(s)\n", result.ImportedHooks)
		}
		return nil
	},
}

var projectListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all initialized projects",
	RunE: func(cmd *cobra.Command, args []string) error {
		// Scan common parent directories
		home, _ := os.UserHomeDir()
		parentDirs := []string{
			filepath.Join(home, "Work"),
			filepath.Join(home, "Projects"),
			filepath.Join(home, "Repositories"),
			filepath.Join(home, "repos"),
			filepath.Join(home, "src"),
		}

		results, err := commands.ProjectListScan(parentDirs)
		if err != nil {
			return err
		}

		if len(results) == 0 {
			fmt.Println("No initialized projects found.")
			fmt.Println("Scanned:", strings.Join(parentDirs, ", "))
			return nil
		}

		fmt.Printf("Found %d managed project(s):\n\n", len(results))
		for _, r := range results {
			profile := r.Profile
			if profile == "" {
				profile = "(base)"
			}
			fmt.Printf("  %s  [%s]\n", r.Path, profile)
		}
		return nil
	},
}

var projectRemoveYes bool

var projectRemoveCmd = &cobra.Command{
	Use:   "remove [path]",
	Short: "Remove claude-sync management from a project",
	Long: `Remove claude-sync management from a project directory.

This deletes .claude/.claude-sync.yaml but leaves settings.local.json as-is.
Managed keys will remain but will no longer be updated by pull.`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		projectDir := "."
		if len(args) > 0 {
			projectDir = args[0]
		}
		projectDir, _ = filepath.Abs(projectDir)

		if !projectRemoveYes {
			var confirm bool
			err := huh.NewForm(
				huh.NewGroup(
					huh.NewConfirm().
						Title(fmt.Sprintf("Remove claude-sync management from %s?", projectDir)).
						Affirmative("Yes, remove").
						Negative("Cancel").
						Value(&confirm),
				),
			).Run()
			if err != nil || !confirm {
				fmt.Println("Cancelled.")
				return nil
			}
		}

		if err := commands.ProjectRemove(projectDir); err != nil {
			return err
		}
		fmt.Printf("Removed claude-sync management from %s\n", projectDir)
		fmt.Println("  settings.local.json left as-is")
		return nil
	},
}

func promptProjectProfile(available []string, active string) (string, error) {
	options := make([]huh.Option[string], 0, len(available)+1)
	for _, name := range available {
		label := capitalize(name)
		if name == active {
			label += " (active)"
		}
		options = append(options, huh.NewOption(label, name))
	}
	options = append(options, huh.NewOption("No profile — use base only", ""))

	var choice string
	fmt.Println()
	err := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Which profile should this project use?").
				Options(options...).
				Value(&choice),
		),
	).Run()
	if err != nil {
		return "", err
	}
	return choice, nil
}

func declineProject(projectDir string) error {
	fmt.Printf("Declined claude-sync management for %s\n", projectDir)
	fmt.Println("  Future pulls will skip this project.")
	// Write a minimal declined config
	os.MkdirAll(filepath.Join(projectDir, ".claude"), 0755)
	data := []byte("version: \"1.0.0\"\ndeclined: true\n")
	return os.WriteFile(filepath.Join(projectDir, ".claude", ".claude-sync.yaml"), data, 0644)
}

func init() {
	projectInitCmd.Flags().StringVar(&projectInitProfile, "profile", "", "Profile to use (skip picker)")
	projectInitCmd.Flags().StringVar(&projectInitKeys, "keys", "", "Comma-separated keys to project (default: hooks,permissions)")
	projectInitCmd.Flags().BoolVar(&projectInitYes, "yes", false, "Non-interactive mode")
	projectInitCmd.Flags().BoolVar(&projectInitDecline, "decline", false, "Decline management (future pulls skip)")

	projectRemoveCmd.Flags().BoolVarP(&projectRemoveYes, "yes", "y", false, "Skip confirmation prompt")

	projectCmd.AddCommand(projectInitCmd)
	projectCmd.AddCommand(projectListCmd)
	projectCmd.AddCommand(projectRemoveCmd)
}
