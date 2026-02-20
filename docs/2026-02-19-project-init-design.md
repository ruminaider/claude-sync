# Project Initialization for claude-sync

## Problem

Claude Code bug [#19487](https://github.com/anthropics/claude-code/issues/19487): when a project has `.claude/settings.local.json`, it overwrites global `~/.claude/settings.json` instead of deep merging. This causes hooks, permissions, and potentially other keys defined in global settings to be lost in project sessions.

Today, claude-sync manages global settings (`~/.claude/settings.json`) but has no awareness of per-project `.claude/settings.local.json` files. Users must manually duplicate hooks and permissions into each project, which is fragile and doesn't scale.

## Solution

Extend claude-sync with project-level initialization that:

1. Manages per-project `.claude/settings.local.json` files
2. Projects resolved configuration (hooks, permissions) from profiles into project settings
3. Captures project-specific overrides (accumulated "Always allow" entries, project MCP servers)
4. Integrates with the existing push/pull lifecycle

## Architecture: Project Mappings

Each initialized project gets a `.claude/.claude-sync.yaml` config file that references a profile and stores project-specific overrides. This file travels with the project (survives renames/moves).

### Resolution Order

```
base config.yaml → machine profile → project overrides → settings.local.json
```

### Project Config File

**Location:** `<project>/.claude/.claude-sync.yaml`

```yaml
version: "1.0.0"
profile: work
initialized: "2026-02-19T10:30:00Z"

# Which keys claude-sync writes into settings.local.json
projected_keys:
  - hooks
  - permissions

# Project-specific overrides layered on top of profile
overrides:
  permissions:
    add_allow:
      - "mcp__evvy_db__query"
      - "mcp__render__list_services"
      - "Bash(docker compose:*)"
  hooks: {}    # inherit from profile
  claude_md:
    add:
      - "evvy-conventions"    # fragment name
  mcp:
    add:
      evvy-db:
        command: "python3"
        args: ["/path/to/evvy-db-mcp/server.py"]
```

This file is gitignore-friendly but CAN be committed for team sharing (v2).

## Command Changes

### Renamed Commands

| Current | New | Purpose |
|---------|-----|---------|
| `claude-sync init` | `claude-sync config create` | Create new global config |
| `claude-sync join` | `claude-sync config join` | Join existing config repo |

`claude-sync init` remains as a hidden alias with deprecation notice.

### New Commands

#### `claude-sync project init [path]`

Initialize a project's settings.local.json from a profile.

**Flow:**

1. Detect existing `.claude/settings.local.json`
2. If exists, import entries not in resolved profile as project overrides
3. Interactive profile picker (or `--profile <name>`)
4. Interactive projected_keys picker (default: `[hooks, permissions]`)
5. Show diff of what will change
6. Confirm, then:
   - Write `.claude/.claude-sync.yaml`
   - Regenerate settings.local.json (managed keys from profile + overrides, unmanaged keys preserved)
   - Add `.claude-sync.yaml` to `.gitignore` if not already

**Import logic (existing settings.local.json):**
- Permissions in file but NOT in resolved profile -> stored as `overrides.permissions.add_allow`
- Hooks in file but NOT in resolved profile -> stored as `overrides.hooks.add`
- Unmanaged keys (enabledMcpjsonServers, enabledPlugins, outputStyle) -> preserved in settings.local.json, not tracked by claude-sync

**Flags:**
- `--profile <name>` — skip profile picker
- `--keys hooks,permissions` — skip key picker
- `--yes` — non-interactive, accept defaults

#### `claude-sync project list`

List all initialized projects by scanning for `.claude-sync.yaml` files.

#### `claude-sync project remove [path]`

Remove claude-sync management from a project:
- Delete `.claude/.claude-sync.yaml`
- Leave settings.local.json as-is (managed keys remain but are no longer updated)
- Confirm before proceeding

## Pull/Push Extensions

### Pull

When `claude-sync pull` runs, it checks if CWD (or parent) has `.claude/.claude-sync.yaml`. If found:

1. Resolve: base config -> profile -> project overrides
2. For each `projected_key`:
   - Read current settings.local.json
   - Replace the managed key with the resolved value
   - Preserve unmanaged keys
3. Write settings.local.json

If not in a project directory, pull behaves as today (global only).

### Push

When `claude-sync push` runs in a project directory:

1. Read settings.local.json
2. For each `projected_key`, diff against resolved profile:
   - New permissions (from "Always allow" clicks) -> prompt to save to overrides
   - Modified hooks -> prompt to save to overrides
3. Update `.claude/.claude-sync.yaml` overrides
4. Commit to claude-sync repo + push

### SessionStart Detection

The existing SessionStart hook runs `claude-sync pull --auto`. Extended behavior:

1. CWD has `.claude-sync.yaml` -> apply project settings (normal pull)
2. CWD has settings.local.json but NO `.claude-sync.yaml` -> prompt:
   ```
   This project has settings.local.json but isn't managed by claude-sync.
   Initialize? (y/n/never)
   ```
3. "never" -> write `.claude-sync.yaml` with `declined: true`

## Per-Project CLAUDE.md Fragments

Project overrides can add CLAUDE.md fragments specific to that project:

```yaml
overrides:
  claude_md:
    add:
      - "evvy-conventions"    # references a fragment in ~/.claude-sync/claude-md/
      - "django-testing"
```

During pull, the assembled CLAUDE.md includes: base fragments + profile fragments + project fragments.

## Per-Project MCP Server Management

Project overrides can add/remove MCP servers:

```yaml
overrides:
  mcp:
    add:
      evvy-db:
        command: "python3"
        args: ["/path/to/server.py"]
    remove:
      - "unused-server"
```

This extends the existing `mcp_metadata.source_project` infrastructure. During pull, project MCP servers are written to `<project>/.mcp.json`.

## Conflict Resolution

### Strategy: YAML-Aware 3-Way Merge + Interactive Resolution

When `git pull --rebase` encounters conflicts in the claude-sync repo:

#### Step 1: Attempt YAML-aware auto-merge

| Scenario | Behavior | Rationale |
|----------|----------|-----------|
| Both add permissions to `allow` | **Auto-merge (union)** | Additive, no conflict |
| Both add hooks with different matchers | **Auto-merge (union)** | Different tools |
| Both add different MCP servers | **Auto-merge (union)** | Additive |
| Both add CLAUDE.md fragments | **Auto-merge (union)** | Additive |
| Both add different keybindings | **Auto-merge (union)** | Different keys |
| One adds, other removes same item | **Conflict** | Intent disagreement |
| Both modify same setting key | **Conflict** | Value disagreement |
| Both modify same hook (same matcher) | **Conflict** | Behavior disagreement |
| Both modify same keybinding | **Conflict** | Value disagreement |
| One removes, other modifies | **Conflict** | Structural disagreement |
| Config version mismatch | **Conflict** | Schema disagreement |

Rule: **additions auto-merge; modifications and deletions require human judgment.**

#### Step 2: Interactive resolution for true conflicts

If auto-merge fails, present interactive resolution immediately (during SessionStart pull):

```
Conflict in config.yaml: defaultMode
  Local:  'plan'
  Remote: 'acceptEdits'
  > Accept remote
  > Keep local
  > Defer (apply remote for now, resolve later)
```

If deferred:
- Remote version applied (sessions must start)
- Local changes saved to `~/.claude-sync/conflicts/<timestamp>.yaml`
- `claude-sync push` BLOCKED until resolved
- `claude-sync status` shows conflict warning
- SessionStart shows persistent reminder
- `claude-sync conflicts resolve` for interactive resolution

#### Principles

1. **Sessions always start** — conflicts never block session initialization
2. **Push blocked until resolved** — prevents ignoring conflicts indefinitely
3. **Auto-merge for additive changes** — most common case (permission accumulation) is frictionless
4. **Interactive for true conflicts** — user judgment required for modifications/deletions

## File Ownership Model

claude-sync owns the projected keys in settings.local.json:

- **Managed keys** (in `projected_keys`): hooks, permissions — regenerated from profile + overrides on each pull
- **Unmanaged keys**: enabledMcpjsonServers, enabledPlugins, outputStyle, enableAllProjectMcpServers — preserved untouched

"Always allow" clicks add permissions to settings.local.json. These are captured by push and stored as project overrides, then re-applied on pull. This mirrors how claude-sync handles global settings today.

## Edge Cases

| Scenario | Behavior |
|----------|----------|
| Project path moves/renames | `.claude-sync.yaml` travels with it |
| settings.local.json deleted | Regenerated on next pull |
| Pull outside any project | Global-only, unchanged |
| Two projects share same profile | Each has own overrides |
| User declines init | `.claude-sync.yaml` with `declined: true` |
| claude-sync not initialized globally | `project init` fails: "Run `claude-sync config create` first" |
| Unmanaged keys in settings.local.json | Preserved untouched |
| Profile deleted after project init | Pull fails with clear error: "Profile 'work' not found" |

## Implementation Phases

### Phase 1: Core project init
- `claude-sync config create` (rename from `init`)
- `claude-sync project init` with import + profile selection
- `claude-sync project list`
- `claude-sync project remove`
- Pull extension: detect `.claude-sync.yaml`, apply managed keys
- Push extension: detect project drift, save to overrides

### Phase 2: Extended project features
- Per-project CLAUDE.md fragments
- Per-project MCP server management
- SessionStart auto-detection of uninitialized projects

### Phase 3: Conflict resolution
- YAML-aware 3-way merge
- Interactive conflict resolution at session start
- `claude-sync conflicts` command
- Push blocking on unresolved conflicts

### Phase 4 (deferred): Team sharing
- Committing `.claude-sync.yaml` to project git repo
- Multi-user contribution to same config repo
- Profile inheritance chains
