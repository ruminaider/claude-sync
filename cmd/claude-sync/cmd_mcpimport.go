package main

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/ruminaider/claude-sync/internal/commands"
	"github.com/ruminaider/claude-sync/internal/paths"
	"github.com/ruminaider/claude-sync/internal/profiles"
	"github.com/spf13/cobra"
)

var (
	mcpImportFrom    string
	mcpImportProfile string
)

var mcpImportCmd = &cobra.Command{
	Use:   "import",
	Short: "Import MCP servers from a project .mcp.json file",
	Long:  "Scan a project's .mcp.json file, detect secrets, and import selected servers into your sync config.",
	RunE: func(cmd *cobra.Command, args []string) error {
		syncDir := paths.SyncDir()
		if _, err := os.Stat(syncDir); os.IsNotExist(err) {
			return fmt.Errorf("claude-sync not initialized. Run 'claude-sync init' first")
		}

		// Phase 1: Scan the source file.
		scan, err := commands.MCPImportScan(mcpImportFrom)
		if err != nil {
			return err
		}

		// Display discovered servers.
		serverNames := make([]string, 0, len(scan.Servers))
		for name := range scan.Servers {
			serverNames = append(serverNames, name)
		}
		sort.Strings(serverNames)

		fmt.Printf("Found %d MCP server(s) in %s:\n", len(serverNames), scan.SourcePath)
		for _, name := range serverNames {
			fmt.Printf("  - %s\n", name)
		}
		fmt.Println()

		// Phase 2: Show secret warnings and confirm replacement.
		servers := scan.Servers
		secretsReplaced := 0

		if len(scan.Secrets) > 0 {
			fmt.Printf("Detected %d secret(s):\n", len(scan.Secrets))
			for _, s := range scan.Secrets {
				// Mask the value for display.
				masked := maskSecret(s.Value)
				fmt.Printf("  - %s.env.%s = %s (%s)\n", s.ServerName, s.EnvKey, masked, s.Reason)
			}
			fmt.Println()

			var replaceSecrets bool
			err := huh.NewForm(
				huh.NewGroup(
					huh.NewConfirm().
						Title("Replace detected secrets with ${ENV_VAR} references?").
						Description("Recommended: keeps secrets out of your sync repo").
						Value(&replaceSecrets),
				),
			).Run()
			if err != nil {
				return err
			}

			if replaceSecrets {
				servers = commands.ReplaceSecrets(servers, scan.Secrets)
				secretsReplaced = len(scan.Secrets)
				fmt.Printf("Replaced %d secret(s) with env var references.\n\n", secretsReplaced)
			}
		}

		// Phase 3: Pick which servers to import.
		if len(serverNames) > 1 {
			var strategy string
			options := []huh.Option[string]{
				huh.NewOption(fmt.Sprintf("Include all %d server(s)", len(serverNames)), "all"),
				huh.NewOption("Choose which to include", "some"),
			}
			err := huh.NewForm(
				huh.NewGroup(
					huh.NewSelect[string]().
						Title("Which servers do you want to import?").
						Options(options...).
						Value(&strategy),
				),
			).Run()
			if err != nil {
				return err
			}

			if strategy == "some" {
				selected, err := runPicker("Select MCP servers to import:", serverNames)
				if err != nil {
					return err
				}
				if len(selected) == 0 {
					fmt.Println("No servers selected. Nothing to import.")
					return nil
				}

				// Filter servers to only selected.
				selectedSet := make(map[string]bool, len(selected))
				for _, s := range selected {
					selectedSet[s] = true
				}
				filteredServers := make(map[string]json.RawMessage, len(selected))
				for name, raw := range servers {
					if selectedSet[name] {
						filteredServers[name] = raw
					}
				}
				servers = filteredServers
				serverNames = selected
			}
		}

		// Phase 4: Ask about project path hint.
		var projectPath string
		var useProjectPath bool
		err = huh.NewForm(
			huh.NewGroup(
				huh.NewConfirm().
					Title("Tag these servers with a project path?").
					Description("On pull, servers will be written to the project's .mcp.json instead of global").
					Value(&useProjectPath),
			),
		).Run()
		if err != nil {
			// Esc = skip project path
			useProjectPath = false
		}

		if useProjectPath {
			// Default to the source file's parent directory.
			defaultPath := sourceDirFromPath(mcpImportFrom)
			projectPath = defaultPath
			err := huh.NewForm(
				huh.NewGroup(
					huh.NewInput().
						Title("Project path:").
						Value(&projectPath),
				),
			).Run()
			if err != nil {
				projectPath = ""
			}
		}

		// Phase 5: Choose target (base config or profile).
		profile := mcpImportProfile
		if profile == "" {
			profileNames, _ := profiles.ListProfiles(syncDir)
			if len(profileNames) > 0 {
				options := make([]huh.Option[string], 0, len(profileNames)+1)
				options = append(options, huh.NewOption("Base config (shared by all profiles)", ""))
				for _, name := range profileNames {
					options = append(options, huh.NewOption("Profile: "+name, name))
				}

				err := huh.NewForm(
					huh.NewGroup(
						huh.NewSelect[string]().
							Title("Import servers to:").
							Options(options...).
							Value(&profile),
					),
				).Run()
				if err != nil {
					profile = "" // default to base
				}
			}
		}

		// Phase 6: Import.
		result, err := commands.MCPImport(commands.MCPImportOptions{
			SyncDir:     syncDir,
			Servers:     servers,
			Profile:     profile,
			ProjectPath: projectPath,
		})
		if err != nil {
			return err
		}

		// Display summary.
		fmt.Println()
		fmt.Printf("Imported %d MCP server(s): %s\n", len(result.Imported), strings.Join(result.Imported, ", "))
		if secretsReplaced > 0 {
			fmt.Printf("Replaced %d secret(s) with ${ENV_VAR} references\n", secretsReplaced)
		}
		if result.TargetProfile != "" {
			fmt.Printf("Target: profile %q\n", result.TargetProfile)
		}
		if result.ProjectPath != "" {
			fmt.Printf("Project path: %s (will route to project .mcp.json on pull)\n", result.ProjectPath)
		}

		return nil
	},
}

// maskSecret shows the first 4 characters of a secret, then asterisks.
func maskSecret(value string) string {
	if len(value) <= 4 {
		return "****"
	}
	return value[:4] + strings.Repeat("*", min(len(value)-4, 12))
}

// sourceDirFromPath extracts the directory path from a file path,
// stripping the .mcp.json filename and any leading dot.
func sourceDirFromPath(path string) string {
	dir := path
	// Remove the filename if it ends with .mcp.json
	if strings.HasSuffix(dir, ".mcp.json") {
		dir = dir[:len(dir)-len(".mcp.json")]
	}
	// Remove trailing slash/dot
	dir = strings.TrimRight(dir, "/.")
	return dir
}

func init() {
	mcpImportCmd.Flags().StringVar(&mcpImportFrom, "from", "", "Path to .mcp.json file to import from (required)")
	mcpImportCmd.MarkFlagRequired("from")
	mcpImportCmd.Flags().StringVar(&mcpImportProfile, "profile", "", "Target profile (default: base config)")
	mcpCmd.AddCommand(mcpImportCmd)
}
