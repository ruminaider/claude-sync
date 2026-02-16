package approval

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"go.yaml.in/yaml/v3"
)

// ConfigChanges represents the set of changes detected from a pull.
type ConfigChanges struct {
	Settings       map[string]any
	Permissions    *PermissionChanges
	HasHookChanges bool
	HasMCPChanges  bool
	ClaudeMD       []string // changed fragment names
	Keybindings    bool
}

// PermissionChanges holds the permission rules that changed.
type PermissionChanges struct {
	Allow []string
	Deny  []string
}

// Change represents a single classified change.
type Change struct {
	Category    string // "permissions", "hooks", "mcp", "settings", "claude_md", "keybindings"
	Description string
}

// ClassifiedChanges splits changes into safe (auto-apply) and high-risk (requires approval).
type ClassifiedChanges struct {
	Safe     []Change
	HighRisk []Change
}

// Classify categorizes config changes into safe and high-risk buckets.
// Safe: settings, claude_md, keybindings. High-risk: permissions, hooks, mcp.
func Classify(changes ConfigChanges) ClassifiedChanges {
	var result ClassifiedChanges

	// Settings → safe
	for key := range changes.Settings {
		result.Safe = append(result.Safe, Change{
			Category:    "settings",
			Description: fmt.Sprintf("setting %q changed", key),
		})
	}

	// ClaudeMD → safe
	for _, name := range changes.ClaudeMD {
		result.Safe = append(result.Safe, Change{
			Category:    "claude_md",
			Description: fmt.Sprintf("CLAUDE.md fragment %q changed", name),
		})
	}

	// Keybindings → safe
	if changes.Keybindings {
		result.Safe = append(result.Safe, Change{
			Category:    "keybindings",
			Description: "keybindings changed",
		})
	}

	// Permissions → high-risk
	if changes.Permissions != nil {
		for _, rule := range changes.Permissions.Allow {
			result.HighRisk = append(result.HighRisk, Change{
				Category:    "permissions",
				Description: fmt.Sprintf("allow rule added: %s", rule),
			})
		}
		for _, rule := range changes.Permissions.Deny {
			result.HighRisk = append(result.HighRisk, Change{
				Category:    "permissions",
				Description: fmt.Sprintf("deny rule added: %s", rule),
			})
		}
	}

	// Hooks → high-risk
	if changes.HasHookChanges {
		result.HighRisk = append(result.HighRisk, Change{
			Category:    "hooks",
			Description: "hooks changed",
		})
	}

	// MCP → high-risk
	if changes.HasMCPChanges {
		result.HighRisk = append(result.HighRisk, Change{
			Category:    "mcp",
			Description: "MCP server configuration changed",
		})
	}

	return result
}

// PendingChanges represents deferred high-risk changes awaiting approval.
type PendingChanges struct {
	PendingSince string                     `yaml:"pending_since,omitempty"`
	Commit       string                     `yaml:"commit,omitempty"`
	Permissions  *PendingPermissions        `yaml:"permissions,omitempty"`
	MCP          map[string]json.RawMessage `yaml:"mcp,omitempty"`
	Hooks        map[string]json.RawMessage `yaml:"hooks,omitempty"`
}

// PendingPermissions holds pending permission changes.
type PendingPermissions struct {
	Allow []string `yaml:"allow,omitempty"`
	Deny  []string `yaml:"deny,omitempty"`
}

// IsEmpty returns true if there are no pending changes.
func (p PendingChanges) IsEmpty() bool {
	if p.Commit != "" {
		return false
	}
	if p.Permissions != nil && (len(p.Permissions.Allow) > 0 || len(p.Permissions.Deny) > 0) {
		return false
	}
	if len(p.MCP) > 0 {
		return false
	}
	if len(p.Hooks) > 0 {
		return false
	}
	return true
}

const pendingFile = "pending-changes.yaml"

// WritePending writes pending changes to syncDir/pending-changes.yaml.
func WritePending(syncDir string, p PendingChanges) error {
	data, err := yaml.Marshal(p)
	if err != nil {
		return fmt.Errorf("marshaling pending changes: %w", err)
	}
	path := filepath.Join(syncDir, pendingFile)
	return os.WriteFile(path, data, 0644)
}

// ReadPending reads pending changes from syncDir/pending-changes.yaml.
// Returns empty PendingChanges (with IsEmpty() = true) if file doesn't exist.
func ReadPending(syncDir string) (PendingChanges, error) {
	path := filepath.Join(syncDir, pendingFile)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return PendingChanges{}, nil
		}
		return PendingChanges{}, fmt.Errorf("reading pending changes: %w", err)
	}
	var p PendingChanges
	if err := yaml.Unmarshal(data, &p); err != nil {
		return PendingChanges{}, fmt.Errorf("parsing pending changes: %w", err)
	}
	return p, nil
}

// ClearPending removes the pending-changes.yaml file.
// No error if the file doesn't exist.
func ClearPending(syncDir string) error {
	path := filepath.Join(syncDir, pendingFile)
	err := os.Remove(path)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("clearing pending changes: %w", err)
	}
	return nil
}
