---
name: using-claude-sync
description: Use when syncing Claude Code configuration, pushing or pulling settings, forking plugins, resolving sync conflicts, or understanding how claude-sync manages plugins, commands, skills, hooks, MCP servers, and CLAUDE.md across machines
---

# Using claude-sync

## Overview

claude-sync is a declarative, git-backed config synchronization tool for Claude Code. It maintains a config repo at `~/.claude-sync/` (remote on GitHub) that tracks plugins, settings, hooks, permissions, CLAUDE.md fragments, MCP servers, keybindings, commands, and skills. Changes flow bidirectionally: `push` captures local state, `pull` applies remote state.

## What Gets Synced

| Category | Config Key | Local Location |
|----------|-----------|----------------|
| Plugins (upstream) | `plugins.upstream` | `~/.claude/plugins/installed_plugins.json` |
| Plugins (pinned) | `plugins.pinned` | Same, with version lock |
| Plugins (forked) | `plugins.forked` | `~/.claude-sync/plugins/<name>/` |
| Settings | `settings` | `~/.claude/settings.json` |
| Hooks | `hooks` | `~/.claude/settings.json` (hooks section) |
| Permissions | `permissions` | `~/.claude/settings.json` (allow/deny) |
| CLAUDE.md | `claude_md.include` | `~/.claude/CLAUDE.md` (assembled from fragments) |
| MCP Servers | `mcp` | `~/.claude/.mcp.json` |
| Keybindings | `keybindings` | `~/.claude/keybindings.json` |
| Commands | `commands` | `~/.claude/commands/*.md` |
| Skills | `skills` | `~/.claude/skills/*/SKILL.md` |

## Plugin Sync Model

Three categories with different behaviors:

| Category | Tracked In | Install Source | Updates |
|----------|-----------|---------------|---------|
| **Upstream** | `plugins.upstream` | Marketplace (remote) | Auto-updated on pull |
| **Pinned** | `plugins.pinned` | Marketplace at version | Locked until unpin |
| **Forked** | `plugins.forked` | `~/.claude-sync/plugins/` | Manual — you own it |

**Key rule:** Upstream plugins are installed from their marketplace. Forked plugins are installed from the config repo's `plugins/` directory. Never edit upstream plugins directly — fork first.

## Forked Plugin Workflow

This is the workflow agents get wrong most often.

### To modify a plugin:

```bash
# 1. Fork it (copies from cache to config repo)
claude-sync fork plugin-name@marketplace

# 2. Edit the SOURCE in the config repo
#    NOT ~/.claude/plugins/ — that's the installed copy
vim ~/.claude-sync/plugins/plugin-name/hooks/hook.py

# 3. Push changes (commits + pushes config repo)
claude-sync push
```

### To return to upstream:

```bash
claude-sync unfork plugin-name@marketplace
```

**The critical mistake:** Editing files in `~/.claude/plugins/` or the plugin cache. Those are installed copies — changes are lost on next pull. Always edit in `~/.claude-sync/plugins/<name>/`.

## Common Operations

| Operation | Command | What It Does |
|-----------|---------|-------------|
| Check status | `claude-sync status` | Shows pending changes, diffs |
| Pull remote | `claude-sync pull` | Fetches git + applies config to local Claude Code |
| Push local | `claude-sync push` | Scans local changes, commits, pushes to remote |
| Fork plugin | `claude-sync fork name@mkt` | Copies plugin to config repo for editing |
| Unfork plugin | `claude-sync unfork name@mkt` | Returns to upstream tracking |
| Pin version | `claude-sync pin name version` | Locks plugin at specific version |
| Unpin | `claude-sync unpin name` | Removes version lock |
| Update config | `claude-sync config update` | Re-scans local Claude Code into config.yaml |

**Auto-sync:** A SessionStart hook runs `claude-sync pull --auto` at the start of each session, keeping config current.

## Pull Behavior: Local Modification Protection

Pull protects locally-modified commands and skills from being overwritten. It tracks content hashes (`.content_hashes.json`) to detect local edits:

- **File doesn't exist locally** — copied from config repo, hash recorded
- **File matches stored hash** — safe to overwrite (no local edits), updated
- **File differs from stored hash** — **skipped** (local modification preserved)

On next push, local modifications are captured and sent to the config repo.

## MCP Server Secrets

**Never hardcode secrets** in MCP server configs. Use `${ENV_VAR}` references:

```yaml
mcp:
  slack:
    env:
      SLACK_TOKEN: "${SLACK_TOKEN}"  # Actual value in ~/.zshrc
```

Push auto-detects and replaces secrets with `${VAR}` references. Pull expands them from the shell environment.

## Common Mistakes

| Mistake | Fix |
|---------|-----|
| Editing `~/.claude/plugins/` directly | Fork first, edit in `~/.claude-sync/plugins/` |
| Hardcoding API keys in MCP config | Use `${ENV_VAR}` references |
| Running push without checking status | Run `claude-sync status` first |
| Pushing with dirty working tree | Commit or stash in `~/.claude-sync/` first |
| Forgetting to push after forking | `claude-sync fork` + edit + `claude-sync push` |
| Editing commands during active sync | Pull may overwrite; edits are hash-protected but time your changes between syncs |
