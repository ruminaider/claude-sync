# ADR-0001: Per-Project Settings Management via Layered Configuration

**Status:** Accepted
**Date:** 2026-02-19
**Authors:** albertgwo
**Deciders:** albertgwo

## Context

### The Problem

Claude Code [bug #19487](https://github.com/anthropics/claude-code/issues/19487) causes project-level `.claude/settings.local.json` to **overwrite** global `~/.claude/settings.json` instead of deep merging. When a project has any `settings.local.json` — even one containing only permissions — hooks, MCP server settings, and other keys defined in the global settings file are silently lost for that session.

Before this decision, users worked around the bug with a `sync-hooks.py` script that duplicated the `hooks` section from global settings into each project's `settings.local.json`. This approach was:

- **Fragile:** A change to global hooks required re-running the script across every project.
- **Incomplete:** Only handled hooks, not permissions, MCP servers, or CLAUDE.md fragments.
- **Manual:** No integration with the existing claude-sync push/pull lifecycle.
- **Unaware of profiles:** Could not differentiate work vs. personal configurations per project.

claude-sync already managed global settings (`~/.claude/settings.json`) with profiles, push/pull, and CLAUDE.md assembly. The gap was per-project awareness.

### Requirements

1. **Automated settings projection:** Resolve hooks, permissions, and other keys from profiles into each project's `settings.local.json` without manual intervention.
2. **Existing settings preservation:** Import permissions accumulated through "Always allow" clicks and preserve unmanaged keys (e.g., `enabledMcpjsonServers`, `outputStyle`).
3. **Drift capture:** When users grant new permissions in Claude Code, capture that drift back into project configuration on push.
4. **Profile-aware:** Different projects can reference different profiles (work, personal).
5. **Non-destructive:** Removing claude-sync management from a project leaves `settings.local.json` intact.
6. **Lifecycle integration:** Work within the existing push/pull/SessionStart flow.

## Decision

### Layered Configuration Chain

We introduce a **four-layer resolution chain** that produces the final `settings.local.json`:

```
base config.yaml -> machine profile -> project overrides -> settings.local.json
```

Each layer adds or removes entries from the one before it. The final output is deterministic and reproducible: given the same inputs, the same `settings.local.json` is always produced.

### Per-Project Config File

Each initialized project gets a `.claude/.claude-sync.yaml` file:

```yaml
version: "1.0.0"
profile: work
initialized: "2026-02-19T10:30:00Z"
projected_keys:
  - hooks
  - permissions
overrides:
  permissions:
    add_allow:
      - "mcp__evvy_db__query"
      - "Bash(docker compose:*)"
  hooks:
    add:
      CustomHook:
        - matcher: "^Bash$"
          hooks:
            - type: command
              command: "/path/to/custom-hook.sh"
  claude_md:
    add:
      - "evvy-conventions"
    remove:
      - "unused-fragment"
  mcp:
    add:
      evvy-db:
        command: "python3"
        args: ["/path/to/server.py"]
    remove:
      - "stale-server"
```

This file:
- **Lives inside the project** at `.claude/.claude-sync.yaml`, traveling with renames and moves.
- **Is gitignored by default** (claude-sync adds it to `.gitignore` during init). Team sharing via committing is a future option (Phase 4).
- **References a profile by name**, not by content — the profile's current state is resolved at pull time.
- **Stores only the delta** from the resolved profile, not the full configuration.

### Projected Keys Model

Projects declare which top-level keys claude-sync manages via `projected_keys`. Currently supported:

| Key | What it manages | Output location |
|-----|----------------|-----------------|
| `hooks` | PreToolUse/PostToolUse hook definitions | `settings.local.json` |
| `permissions` | `allow`/`deny` tool permission lists | `settings.local.json` |
| `claude_md` | CLAUDE.md fragment assembly | `.claude/CLAUDE.md` |
| `mcp` | MCP server configuration | `.mcp.json` |

Keys **not** in `projected_keys` are left untouched — claude-sync does not read or write them. This is the central safety property: projects opt into management per key, and unmanaged keys are preserved as-is.

### Override Pattern

Project overrides follow the same add/remove pattern already established by profiles:

```go
type ProjectOverrides struct {
    Permissions ProjectPermissionOverrides // AddAllow, AddDeny
    Hooks       ProjectHookOverrides       // Add (map), Remove (list)
    ClaudeMD    ProjectClaudeMDOverrides   // Add (list), Remove (list)
    MCP         ProjectMCPOverrides        // Add (map), Remove (list)
}
```

This mirrors `ProfilePermissions`, `ProfileHooks`, `ProfileMCP`, and `ProfileClaudeMD` from the existing profiles package. The deliberate symmetry means merge logic is reusable and the mental model is consistent: profiles and projects both use add/remove directives against a base.

### Import on Init

When `project init` runs on a directory with an existing `settings.local.json`, it performs a **diff-based import**: permissions and hooks present in the file but absent from the resolved profile are captured as project overrides. This avoids data loss during onboarding and means users don't have to manually reconstruct their accumulated "Always allow" permissions.

### Pull Regeneration

On `pull`, managed keys in `settings.local.json` are **fully regenerated** from the resolution chain. This is a **replace, not merge** strategy for managed keys: the resolved value is authoritative. Unmanaged keys are read back and preserved.

This means:
- Stale permissions from a previous profile are automatically cleaned up.
- Manual edits to managed keys in `settings.local.json` are overwritten (by design — use push to capture drift first).

### Push Drift Capture

`push` reads the current `settings.local.json`, diffs it against the resolved profile + existing overrides, and saves any new entries to the project overrides. This handles the "Always allow" workflow: Claude Code adds a permission to the file, push detects it, and stores it as an override so the next pull preserves it.

### Conflict Resolution

A YAML-aware 3-way merge engine handles concurrent config changes:

- **Additions auto-merge (union):** Both sides adding permissions or hooks results in a union. This covers the most common case (permission accumulation across machines).
- **Modifications and deletions conflict:** Both sides modifying the same setting key, or one adding what the other removes, requires human judgment.
- **Sessions always start:** Conflicts are deferred, not blocking. Push is blocked until conflicts are resolved, preventing indefinite avoidance.

Pending conflicts are stored in `~/.claude-sync/conflicts/` as timestamped YAML files.

## Alternatives Considered

### Alternative 1: Symlink-Based Approach

Symlink `settings.local.json` to a centrally managed file.

**Rejected because:**
- Claude Code may not follow symlinks reliably.
- Cannot preserve unmanaged keys (the file is either managed or not).
- "Always allow" clicks would modify the shared file, affecting all projects using that symlink target.
- OS-specific symlink behavior adds complexity.

### Alternative 2: Post-Write Hook on settings.local.json

Use a filesystem watcher or git hook to re-inject global settings after Claude Code overwrites them.

**Rejected because:**
- Race conditions: Claude Code reads the file immediately after writing; re-injection may not be fast enough.
- No profile awareness — would need a separate mechanism to know which profile applies.
- Invasive: requires running a background daemon or modifying git hooks in every project.

### Alternative 3: Patch Claude Code Directly

Contribute a fix to Claude Code upstream (change overwrite to deep merge).

**Not rejected, but deferred:** This is the correct long-term fix, but:
- We have no timeline for when/if the fix would ship.
- Even after a fix, per-project configuration (different hooks per project, project-specific MCP servers) has standalone value.
- claude-sync's resolution chain provides more control than a simple deep merge.

### Alternative 4: Full File Ownership (No Unmanaged Keys)

Have claude-sync own the entire `settings.local.json`, not just projected keys.

**Rejected because:**
- Claude Code writes several keys to this file that users adjust interactively (e.g., `outputStyle`, `enabledPlugins`). Owning the whole file would overwrite these on every pull.
- Projected keys are a strict subset — this limits blast radius and makes the contract clear.

### Alternative 5: Store Overrides in the claude-sync Repo (Not the Project)

Keep project configs in `~/.claude-sync/projects/<hash>.yaml` rather than `<project>/.claude/.claude-sync.yaml`.

**Rejected because:**
- Config doesn't travel with the project on moves/renames.
- Need a path-to-config mapping that breaks on path changes.
- Harder to reason about which config applies where.
- The project-local file makes the relationship explicit and inspectable.

## Consequences

### Positive

- **Solves the #19487 workaround at scale.** Hooks, permissions, CLAUDE.md, and MCP servers are automatically projected into every managed project.
- **Existing permissions preserved.** Init imports accumulated "Always allow" entries as overrides instead of discarding them.
- **Profile flexibility.** Each project can use a different profile. Changing a profile's hooks propagates to all projects using that profile on the next pull.
- **Drift capture.** New permissions granted via Claude Code's UI are captured on push, not lost on the next pull.
- **Non-destructive removal.** `project remove` deletes only `.claude-sync.yaml`; `settings.local.json` continues working with its last-generated values.

### Negative

- **Additional config file per project.** Each managed project gets `.claude/.claude-sync.yaml`. This is gitignored by default but adds to the `.claude/` directory.
- **Pull regenerates managed keys.** Manual edits to managed keys in `settings.local.json` are overwritten. Users must use push to persist changes.
- **New mental model.** Users need to understand the concept of projected keys, managed vs. unmanaged, and the resolution chain. The `project init --keys` flag and import logic help, but there is a learning curve.
- **Concurrent modification risk.** Multiple agents (Wave 4A and 4B) modified `ApplyProjectSettings` concurrently during development, requiring careful merge. Future changes to this function should be serialized.

### Risks

- **Claude Code fixes #19487.** If Claude Code implements proper deep merging, the projected keys for `hooks` and `permissions` become redundant. However, the per-project CLAUDE.md and MCP features have independent value, and the override pattern (project-specific permissions beyond the profile) remains useful.
- **settings.local.json schema changes.** If Claude Code changes the structure of `settings.local.json`, the projected keys logic may need updating. The switch-based approach in `ApplyProjectSettings` isolates each key's handling.

## Technical Details

### Package Structure

| Package | Responsibility |
|---------|---------------|
| `internal/project` | `ProjectConfig` types, YAML read/write, `FindProjectRoot()` |
| `internal/commands` | `ProjectInit`, `ProjectPush`, `ApplyProjectSettings`, `ResolveWithProfile`, conflict management |
| `internal/merge` | 3-way merge engine (permissions, settings, hooks) |
| `cmd/claude-sync` | CLI wiring (`project init/list/remove`, `conflicts`) |

### Resolution Implementation

`ResolveWithProfile()` merges base config with a named profile using the existing `profiles.Merge*()` functions, producing a `ResolvedConfig` with all five surfaces (Hooks, Permissions, Settings, ClaudeMD, MCP).

`ApplyProjectSettings()` takes the resolved config, applies project overrides (add/remove), and writes managed keys to the appropriate output files. The function operates as a pure transformation: read current state, compute desired state, write.

### Decline Mechanism

Users can decline management (`project init --decline`) which writes a minimal `.claude-sync.yaml` with `declined: true`. Future pulls and SessionStart detection skip declined projects. This prevents repeated prompting.

### SessionStart Integration

When `pull --auto` runs in a directory with `settings.local.json` but no `.claude-sync.yaml`, it sets `ProjectUnmanagedDetected = true` in the result. The CLI renders a suggestion to run `project init`. This is informational only — it does not block the session.

## Related Documents

- [Design Document](../2026-02-19-project-init-design.md) — Full design with edge cases and phasing
- [Implementation Plan](../plans/2026-02-19-project-init-plan.md) — 14-task plan with dependency graph
- [Claude Code Bug #19487](https://github.com/anthropics/claude-code/issues/19487) — Upstream bug this works around
