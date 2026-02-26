package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/ruminaider/claude-sync/internal/commands"
)

// printPullResult displays the results of a pull operation.
func printPullResult(result *commands.PullResult) {
	if len(result.Installed) > 0 {
		fmt.Printf("\n✓ %d plugin(s) installed\n", len(result.Installed))
	}
	if len(result.Updated) > 0 {
		fmt.Printf("✓ %d plugin(s) updated\n", len(result.Updated))
	}

	if len(result.SettingsApplied) > 0 {
		fmt.Printf("✓ Settings applied: %s\n", strings.Join(result.SettingsApplied, ", "))
	}
	if len(result.HooksApplied) > 0 {
		fmt.Printf("✓ Hooks applied: %s\n", strings.Join(result.HooksApplied, ", "))
	}
	if result.PermissionsApplied {
		fmt.Println("✓ Permissions applied")
	}
	if result.ClaudeMDAssembled {
		fmt.Println("✓ CLAUDE.md assembled from fragments")
	}
	if len(result.MCPApplied) > 0 {
		fmt.Printf("✓ MCP servers applied: %s\n", strings.Join(result.MCPApplied, ", "))
	}
	for projectPath, names := range result.MCPProjectApplied {
		fmt.Printf("✓ MCP servers applied to %s: %s\n", projectPath, strings.Join(names, ", "))
	}
	if len(result.MCPEnvWarnings) > 0 {
		for _, w := range result.MCPEnvWarnings {
			fmt.Fprintf(os.Stderr, "  Warning: %s\n", w)
		}
	}
	if result.KeybindingsApplied {
		fmt.Println("✓ Keybindings applied")
	}

	if len(result.SkippedCategories) > 0 {
		fmt.Printf("  Skipped: %s (per user-preferences.yaml)\n", strings.Join(result.SkippedCategories, ", "))
	}

	allFailed := append(result.Failed, result.UpdateFailed...)
	if len(allFailed) > 0 {
		fmt.Fprintf(os.Stderr, "\n⚠️  %d plugin(s) failed:\n", len(allFailed))
		for _, p := range allFailed {
			fmt.Fprintf(os.Stderr, "  • %s\n", p)
		}
	}

	nothingChanged := len(result.ToInstall) == 0 && len(result.Updated) == 0 && len(result.SettingsApplied) == 0 && len(result.HooksApplied) == 0 &&
		len(result.SkippedCategories) == 0 && !result.PermissionsApplied && !result.ClaudeMDAssembled &&
		len(result.MCPApplied) == 0 && len(result.MCPProjectApplied) == 0 && !result.KeybindingsApplied
	if len(allFailed) > 0 {
		fmt.Fprintf(os.Stderr, "\nSome plugins could not be installed. Check the errors above.\n")
	} else if nothingChanged {
		fmt.Println("Everything up to date.")
	}

	if len(result.PendingHighRisk) > 0 {
		fmt.Printf("\n⚠ %d high-risk change(s) deferred. Run 'claude-sync approve' to apply:\n", len(result.PendingHighRisk))
		for _, c := range result.PendingHighRisk {
			fmt.Printf("  • %s\n", c.Description)
		}
	}

	if len(result.Untracked) > 0 && len(allFailed) == 0 {
		fmt.Printf("\nNote: %d plugin(s) installed locally but not in config:\n", len(result.Untracked))
		for _, p := range result.Untracked {
			fmt.Printf("  • %s\n", p)
		}
		fmt.Println("Run 'claude-sync push' to add them, or keep as local-only.")
	}

	if result.ProjectUnmanagedDetected {
		fmt.Println("\nThis project has settings.local.json but isn't managed by claude-sync.")
		fmt.Println("Run 'claude-sync project init' to sync hooks and permissions.")
	}
}
