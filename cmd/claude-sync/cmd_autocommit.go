package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/ruminaider/claude-sync/internal/commands"
	"github.com/ruminaider/claude-sync/internal/paths"
	"github.com/spf13/cobra"
)

var autoCommitIfChanged bool
var autoCommitForceBase bool

var autoCommitCmd = &cobra.Command{
	Use:   "auto-commit",
	Short: "Auto-commit local changes to sync repo",
	RunE: func(cmd *cobra.Command, args []string) error {
		projectDir := readCWDFromStdin()

		result, err := commands.AutoCommitWithContext(commands.AutoCommitOptions{
			ClaudeDir:  paths.ClaudeDir(),
			SyncDir:    paths.SyncDir(),
			ProjectDir: projectDir,
			ForceBase:  autoCommitForceBase,
		})
		if err != nil {
			return err
		}

		if !result.Changed {
			if !autoCommitIfChanged {
				fmt.Println("No changes detected.")
			}
			return nil
		}

		fmt.Printf("Committed: %s\n", result.CommitMessage)
		return nil
	},
}

// readCWDFromStdin reads the "cwd" field from JSON piped on stdin
// (as provided by Claude Code hooks). Falls back to os.Getwd().
func readCWDFromStdin() string {
	info, err := os.Stdin.Stat()
	if err != nil {
		return cwdFallback()
	}
	// If stdin is a terminal (not piped), skip reading.
	if info.Mode()&os.ModeCharDevice != 0 {
		return cwdFallback()
	}

	data, err := io.ReadAll(os.Stdin)
	if err != nil || len(data) == 0 {
		return cwdFallback()
	}

	var payload struct {
		CWD string `json:"cwd"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return cwdFallback()
	}
	if payload.CWD != "" {
		return payload.CWD
	}
	return cwdFallback()
}

func cwdFallback() string {
	cwd, _ := os.Getwd()
	return cwd
}

func init() {
	autoCommitCmd.Flags().BoolVar(&autoCommitIfChanged, "if-changed", false, "Only output if changes were committed")
	autoCommitCmd.Flags().BoolVar(&autoCommitForceBase, "base", false, "Force changes to base config, ignoring profile")
}
