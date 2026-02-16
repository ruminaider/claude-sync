package commands

import (
	"encoding/json"
	"fmt"

	"github.com/ruminaider/claude-sync/internal/approval"
	"github.com/ruminaider/claude-sync/internal/claudecode"
)

// ApproveResult holds the result of applying pending changes.
type ApproveResult struct {
	PermissionsApplied bool
	MCPApplied         []string
	HooksApplied       []string
}

// Approve reads pending-changes.yaml, applies each pending change, then clears the file.
func Approve(claudeDir, syncDir string) (*ApproveResult, error) {
	pending, err := approval.ReadPending(syncDir)
	if err != nil {
		return nil, fmt.Errorf("reading pending changes: %w", err)
	}

	if pending.IsEmpty() {
		return nil, fmt.Errorf("no pending changes to approve")
	}

	result := &ApproveResult{}

	// Apply pending permissions.
	if pending.Permissions != nil && (len(pending.Permissions.Allow) > 0 || len(pending.Permissions.Deny) > 0) {
		settings, err := claudecode.ReadSettings(claudeDir)
		if err != nil {
			settings = make(map[string]json.RawMessage)
		}

		// Read existing permissions.
		var existingPerms struct {
			Allow []string `json:"allow"`
			Deny  []string `json:"deny"`
		}
		if permRaw, ok := settings["permissions"]; ok {
			json.Unmarshal(permRaw, &existingPerms)
		}

		// Additive merge.
		mergedAllow := appendUniqueApprove(existingPerms.Allow, pending.Permissions.Allow)
		mergedDeny := appendUniqueApprove(existingPerms.Deny, pending.Permissions.Deny)

		permData, err := json.Marshal(map[string]any{
			"allow": mergedAllow,
			"deny":  mergedDeny,
		})
		if err == nil {
			settings["permissions"] = json.RawMessage(permData)
			if err := claudecode.WriteSettings(claudeDir, settings); err != nil {
				return nil, fmt.Errorf("applying permissions: %w", err)
			}
			result.PermissionsApplied = true
		}
	}

	// Apply pending MCP servers.
	if len(pending.MCP) > 0 {
		existing, _ := claudecode.ReadMCPConfig(claudeDir)
		for k, v := range pending.MCP {
			existing[k] = v
		}
		if err := claudecode.WriteMCPConfig(claudeDir, existing); err != nil {
			return nil, fmt.Errorf("applying MCP config: %w", err)
		}
		result.MCPApplied = make([]string, 0, len(pending.MCP))
		for k := range pending.MCP {
			result.MCPApplied = append(result.MCPApplied, k)
		}
	}

	// Apply pending hooks.
	if len(pending.Hooks) > 0 {
		settings, err := claudecode.ReadSettings(claudeDir)
		if err != nil {
			settings = make(map[string]json.RawMessage)
		}

		var existingHooks map[string]json.RawMessage
		if hooksRaw, ok := settings["hooks"]; ok {
			json.Unmarshal(hooksRaw, &existingHooks)
		}
		if existingHooks == nil {
			existingHooks = make(map[string]json.RawMessage)
		}

		for k, v := range pending.Hooks {
			existingHooks[k] = v
			result.HooksApplied = append(result.HooksApplied, k)
		}

		hooksData, err := json.Marshal(existingHooks)
		if err == nil {
			settings["hooks"] = json.RawMessage(hooksData)
			if err := claudecode.WriteSettings(claudeDir, settings); err != nil {
				return nil, fmt.Errorf("applying hooks: %w", err)
			}
		}
	}

	// Clear pending changes.
	if err := approval.ClearPending(syncDir); err != nil {
		return nil, fmt.Errorf("clearing pending: %w", err)
	}

	return result, nil
}

// appendUniqueApprove appends items from add to base without duplicates.
func appendUniqueApprove(base, add []string) []string {
	seen := make(map[string]bool, len(base))
	for _, s := range base {
		seen[s] = true
	}
	result := make([]string, len(base))
	copy(result, base)
	for _, s := range add {
		if !seen[s] {
			seen[s] = true
			result = append(result, s)
		}
	}
	return result
}
