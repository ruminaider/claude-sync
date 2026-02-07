---
name: sync
description: Manage claude-sync configuration
arguments:
  - name: action
    description: "Action: status, pull, apply"
    required: false
    type: string
---

# /sync — Claude-Sync Plugin Commands

You have access to the claude-sync CLI tool for managing plugin synchronization.

## Available Actions

### /sync status (default)
Show the current sync status. Run: `claude-sync status --json`
Parse the JSON output and present a human-readable summary showing:
- Upstream plugins and their versions
- Pinned plugins with their locked versions
- Forked plugins from the sync repo
- Any untracked or missing plugins

### /sync pull
Pull latest configuration from remote. Run: `claude-sync pull`
Report what was installed, updated, or failed.

### /sync apply
Apply pending plugin updates. Run: `claude-sync update`
This reinstalls upstream plugins to get latest versions and syncs forked plugins.
Report results including any failures.

## Instructions

1. If no action is specified, default to `status`
2. Always use the CLI tool via bash — do not modify config files directly
3. For `status`, use `--json` flag and format the output nicely
4. Report errors clearly and suggest next steps
5. If claude-sync is not initialized, guide the user to run `claude-sync init` or `claude-sync join <url>`
