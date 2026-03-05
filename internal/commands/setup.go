package commands

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ruminaider/claude-sync/internal/claudecode"
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

// SetupAutoSyncHooksDescription returns a human-readable description of what hooks will be registered.
func SetupAutoSyncHooksDescription() string {
	return `Auto-sync hooks will be registered in ~/.claude/settings.json:

  PostToolUse (Write|Edit) → claude-sync auto-commit --if-changed

Session lifecycle hooks (SessionStart, SessionEnd, Stop) are handled by the
claude-sync plugin and do not need settings.json entries.
`
}

// SetupAutoSyncHooks registers auto-sync hooks in settings.json.
// It merges alongside existing hooks without overwriting them.
func SetupAutoSyncHooks(claudeDir string) error {
	settings, err := claudecode.ReadSettings(claudeDir)
	if err != nil {
		settings = make(map[string]json.RawMessage)
	}

	// Parse existing hooks.
	var hooks map[string]json.RawMessage
	if hooksRaw, ok := settings["hooks"]; ok {
		if json.Unmarshal(hooksRaw, &hooks) != nil {
			hooks = nil
		}
	}
	if hooks == nil {
		hooks = make(map[string]json.RawMessage)
	}

	type hookAction struct {
		Type    string `json:"type"`
		Command string `json:"command"`
	}
	type hookRule struct {
		Matcher string       `json:"matcher"`
		Hooks   []hookAction `json:"hooks"`
	}

	// Define the hooks we want to register.
	// Session lifecycle hooks (SessionStart, SessionEnd, Stop) are handled by the plugin.
	wantedHooks := map[string]hookRule{
		"PostToolUse": {
			Matcher: "Write|Edit",
			Hooks:   []hookAction{{Type: "command", Command: "claude-sync auto-commit --if-changed"}},
		},
	}

	for eventName, wanted := range wantedHooks {
		var existing []hookRule
		if raw, ok := hooks[eventName]; ok {
			json.Unmarshal(raw, &existing)
		}

		// Check if claude-sync hook already present.
		alreadyPresent := false
		for _, rule := range existing {
			for _, h := range rule.Hooks {
				if strings.Contains(h.Command, "claude-sync") {
					alreadyPresent = true
					break
				}
			}
			if alreadyPresent {
				break
			}
		}

		if alreadyPresent {
			continue
		}

		existing = append(existing, wanted)
		data, err := json.Marshal(existing)
		if err != nil {
			return fmt.Errorf("marshaling hook %s: %w", eventName, err)
		}
		hooks[eventName] = json.RawMessage(data)
	}

	hooksData, err := json.Marshal(hooks)
	if err != nil {
		return fmt.Errorf("marshaling hooks: %w", err)
	}
	settings["hooks"] = json.RawMessage(hooksData)

	return claudecode.WriteSettings(claudeDir, settings)
}
