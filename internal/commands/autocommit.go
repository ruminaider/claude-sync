package commands

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/ruminaider/claude-sync/internal/claudecode"
	"github.com/ruminaider/claude-sync/internal/claudemd"
	"github.com/ruminaider/claude-sync/internal/config"
	"github.com/ruminaider/claude-sync/internal/git"
)

// AutoCommitResult holds the result of an auto-commit operation.
type AutoCommitResult struct {
	Changed       bool
	CommitMessage string
	FilesChanged  []string
}

// AutoCommit checks for local changes to CLAUDE.md, settings, and MCP,
// then creates a local git commit if anything changed. Does NOT push.
func AutoCommit(claudeDir, syncDir string) (*AutoCommitResult, error) {
	if _, err := os.Stat(syncDir); os.IsNotExist(err) {
		return &AutoCommitResult{}, nil
	}

	cfgData, err := os.ReadFile(filepath.Join(syncDir, "config.yaml"))
	if err != nil {
		return nil, fmt.Errorf("reading config: %w", err)
	}
	cfg, err := config.Parse(cfgData)
	if err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}

	var changes []string
	var stagedFiles []string
	configChanged := false

	// Check CLAUDE.md changes.
	claudeMDPath := filepath.Join(claudeDir, "CLAUDE.md")
	if claudeMDData, err := os.ReadFile(claudeMDPath); err == nil {
		reconcileResult, err := claudemd.Reconcile(syncDir, string(claudeMDData))
		if err == nil {
			if len(reconcileResult.Updated) > 0 {
				changes = append(changes, "update "+strings.Join(reconcileResult.Updated, ", "))
				stagedFiles = append(stagedFiles, "claude-md")
			}
			if len(reconcileResult.New) > 0 {
				names := make([]string, len(reconcileResult.New))
				for i, s := range reconcileResult.New {
					names[i] = claudemd.HeaderToFragmentName(s.Header)
					// Write new fragment files.
					claudeMdDir := filepath.Join(syncDir, "claude-md")
					os.MkdirAll(claudeMdDir, 0755)
					claudemd.WriteFragment(claudeMdDir, names[i], s.Content)
				}
				changes = append(changes, "add "+strings.Join(names, ", "))
				stagedFiles = append(stagedFiles, "claude-md")
				// Update config.yaml to include new fragments.
				for _, name := range names {
					cfg.ClaudeMD.Include = append(cfg.ClaudeMD.Include, name)
				}
				configChanged = true
			}
		}
	}

	// Check settings changes.
	settingsRaw, settErr := claudecode.ReadSettings(claudeDir)
	if settErr == nil && cfg.Settings != nil {
		for key, val := range cfg.Settings {
			if raw, ok := settingsRaw[key]; ok {
				var current any
				json.Unmarshal(raw, &current)
				currentJSON, _ := json.Marshal(current)
				cfgJSON, _ := json.Marshal(val)
				if string(currentJSON) != string(cfgJSON) {
					cfg.Settings[key] = current
					configChanged = true
					changes = append(changes, "update setting "+key)
				}
			}
		}
	}

	// Check MCP changes.
	currentMCP, mcpErr := claudecode.ReadMCPConfig(claudeDir)
	if mcpErr == nil && len(currentMCP) > 0 {
		if !jsonMapsEqual(currentMCP, cfg.MCP) {
			cfg.MCP = currentMCP
			configChanged = true
			changes = append(changes, "update MCP servers")
		}
	}

	if len(changes) == 0 {
		return &AutoCommitResult{}, nil
	}

	// Write updated config if needed.
	if configChanged {
		newData, err := config.Marshal(cfg)
		if err != nil {
			return nil, fmt.Errorf("marshaling config: %w", err)
		}
		cfgPath := filepath.Join(syncDir, "config.yaml")
		if err := os.WriteFile(cfgPath, newData, 0644); err != nil {
			return nil, fmt.Errorf("writing config: %w", err)
		}
		stagedFiles = append(stagedFiles, "config.yaml")
	}

	// Stage and commit.
	sort.Strings(stagedFiles)
	// Deduplicate.
	seen := make(map[string]bool)
	var deduped []string
	for _, f := range stagedFiles {
		if !seen[f] {
			seen[f] = true
			deduped = append(deduped, f)
		}
	}

	for _, f := range deduped {
		if err := git.Add(syncDir, f); err != nil {
			return nil, fmt.Errorf("staging %s: %w", f, err)
		}
	}

	commitMsg := "auto: " + strings.Join(changes, ", ")
	if err := git.Commit(syncDir, commitMsg); err != nil {
		return nil, fmt.Errorf("committing: %w", err)
	}

	return &AutoCommitResult{
		Changed:       true,
		CommitMessage: commitMsg,
		FilesChanged:  deduped,
	}, nil
}
