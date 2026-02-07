package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func SetupShellAlias() string {
	shell := os.Getenv("SHELL")
	alias := `alias claude='claude-sync pull --quiet 2>/dev/null; command claude'`

	var rcFile string
	switch {
	case strings.HasSuffix(shell, "zsh"):
		rcFile = filepath.Join(os.Getenv("HOME"), ".zshrc")
	case strings.HasSuffix(shell, "bash"):
		rcFile = filepath.Join(os.Getenv("HOME"), ".bashrc")
	case strings.HasSuffix(shell, "fish"):
		alias = `alias claude 'claude-sync pull --quiet 2>/dev/null; command claude'`
		rcFile = filepath.Join(os.Getenv("HOME"), ".config", "fish", "config.fish")
	default:
		rcFile = "your shell's rc file"
	}

	return fmt.Sprintf(`To ensure Claude Code always starts with fresh config, add this alias:

  %s

Add it to %s, then restart your shell or run:

  source %s
`, alias, rcFile, rcFile)
}
