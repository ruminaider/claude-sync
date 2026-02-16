# Full Configuration Sync Design

**Date:** 2026-02-13
**Status:** Draft

## Problem

claude-sync currently syncs plugins, a subset of settings, and hooks. But Claude Code has many more configuration surfaces that users want portable across machines: global CLAUDE.md instructions, permissions, MCP servers, status line, keybindings, and environment variables. Setting up a new machine requires manual reconfiguration of these surfaces.

## Goal

Extend claude-sync to sync all portable Claude Code configuration, enabling 1:1 machine replication with version control. Users should be able to clone their sync repo on a new machine and have their full Claude Code environment reconstructed — with profile support for work vs personal differentiation.

## Syncable Surfaces

| Surface | Storage in sync repo | Apply target on pull |
|---|---|---|
| Plugins | `config.yaml` plugins section | `claude plugin install` commands |
| Hooks | `config.yaml` hooks section | `~/.claude/settings.json` hooks |
| Settings (model, env, statusLine) | `config.yaml` settings section | `~/.claude/settings.json` |
| CLAUDE.md fragments | `claude-md/*.md` + manifest | `~/.claude/CLAUDE.md` (assembled) |
| Permissions (allow/deny) | `config.yaml` permissions section | `~/.claude/settings.json` permissions |
| MCP servers | `config.yaml` mcp section | `~/.claude/.mcp.json` |
| Keybindings | `config.yaml` keybindings section | `~/.claude/keybindings.json` |

### Not synced (machine-local)

- `settings.local.json` — accumulated permissions beyond base set
- Plugin cache/binaries — reinstalled from marketplace on pull
- Session data, telemetry, history, debug logs
- Project-level `settings.local.json` files
- `pending-changes.yaml` — unapproved high-risk changes (gitignored)

## Config Format

No version bump needed. New sections are purely additive — existing `plugins`, `settings`, and `hooks` fields are unchanged. Config version stays at `1.0.0`.

```yaml
version: "1.0.0"

plugins:
  upstream:
    - beads@beads-marketplace
    - context7@claude-plugins-official
  pinned:
    - superpowers@superpowers-marketplace: "4.1.1"
  forked:
    - custom-tool
  excluded:
    - pyright-lsp@claude-plugins-official

claude_md:
  include:
    - _preamble
    - git-conventions
    - color-preferences
    - subagent-workflow

settings:
  model: claude-opus-4-6
  statusLine:
    type: command
    command: "jq-based-status-line..."
  env:
    CLAUDE_CODE_EXPERIMENTAL_AGENT_TEAMS: "1"

permissions:
  allow:
    - Read
    - Edit
    - Write
    - Glob
    - Grep
    - "Bash(git status*)"
    - "Bash(git log*)"
    - "Bash(git diff*)"
  deny:
    - "Bash(rm -rf *)"
    - "Bash(sudo *)"
    - "Bash(git push --force*)"

hooks:
  PreCompact:
    - matcher: ""
      hooks:
        - type: command
          command: "bd prime"
  SessionStart:
    - matcher: ""
      hooks:
        - type: command
          command: "bd prime"

mcp:
  context7:
    type: stdio
    command: npx
    args: [-y, "@context7/mcp"]

keybindings: {}
```

## Profile System

All syncable surfaces support the same add/remove/override pattern in profiles. This extends the existing profile mechanism (which already handles plugins and hooks) to cover every surface.

```yaml
# profiles/work.yaml
plugins:
  add:
    - greptile@claude-plugins-official
  remove:
    - beads@beads-marketplace

claude_md:
  add:
    - tool-auth-rules
    - work-conventions
  remove:
    - color-preferences

settings:
  model: claude-sonnet-4-20250514

permissions:
  add_allow:
    - "Bash(kubectl *)"
    - "Bash(docker *)"
  add_deny:
    - "Bash(git push --force*)"

hooks:
  add:
    SessionStart:
      - matcher: ""
        hooks:
          - type: command
            command: "work-setup.sh"
  remove:
    PreCompact: true

mcp:
  add:
    linear:
      type: stdio
      command: npx
      args: [-y, "@linear/mcp"]
  remove:
    - context7
```

### Merge order (unchanged)

`config.yaml` (base) → `profiles/<active>.yaml` (overlay) → `user-preferences.yaml` (machine-local)

## CLAUDE.md Fragment System

### Overview

CLAUDE.md is managed as a collection of markdown fragment files in the sync repo. On pull, fragments are assembled into a clean `~/.claude/CLAUDE.md`. On push, changes to CLAUDE.md are disassembled back into fragments.

The agent only ever sees the assembled file — pure markdown with no metadata or markers.

### Repo structure

```
~/.claude-sync/
├── claude-md/
│   ├── manifest.yaml          # Fragment metadata and ordering
│   ├── _preamble.md           # Content before first ## header
│   ├── git-conventions.md
│   ├── color-preferences.md
│   └── subagent-workflow.md
```

### Manifest

```yaml
# claude-md/manifest.yaml
fragments:
  _preamble:
    header: null
    content_hash: "a1b2c3"
  git-conventions:
    header: "Git Conventions"
    content_hash: "d4e5f6"
  color-preferences:
    header: "Color Preferences"
    content_hash: "g7h8i9"
  subagent-workflow:
    header: "Subagent-Driven Development"
    content_hash: "j0k1l2"
order:
  - _preamble
  - git-conventions
  - color-preferences
  - subagent-workflow
```

### Assembly (pull)

1. Resolve active profile's fragment list: base `include` + profile `add` - profile `remove`
2. Read each fragment file in `order` sequence
3. Concatenate into single markdown string
4. Write to `~/.claude/CLAUDE.md`

### Disassembly (push)

1. Split current `~/.claude/CLAUDE.md` on `## ` headers
2. Match each section to known fragments using this priority:
   a. **Exact header match** → update fragment content and content_hash
   b. **Header miss, content >80% similar to a missing fragment** → rename detected, confirm with user
   c. **No match at all** → new section, prompt user to create fragment
   d. **Known fragment not found in file** → prompt user to confirm deletion
3. Update `manifest.yaml`
4. Commit changed fragment files

### Init from existing CLAUDE.md

1. Read `~/.claude/CLAUDE.md`
2. Split by `## ` headers
3. Generate fragment names from headers (lowercase, hyphenated)
4. Content before first `## ` header → `_preamble.md`
5. Show proposed fragments, user confirms or adjusts naming
6. Write fragment files and manifest

### Profile composition

Profiles use `add` and `remove` on fragment names, same as plugins:

```yaml
claude_md:
  add: [tool-auth-rules]      # Fragment must exist in claude-md/
  remove: [color-preferences]  # Excluded from assembly for this profile
```

## Security: Tiered Approval on Pull

### Threat model

Synced configuration from a shared git repo could be modified maliciously. Different surfaces have different risk levels:

| Surface | Risk | Reason |
|---|---|---|
| Permissions (allow/deny) | High | Grants Claude ability to run arbitrary commands |
| MCP servers | High | Adds arbitrary external tool access |
| Hooks | High | Runs arbitrary shell commands on events |
| CLAUDE.md fragments | Medium | Behavioral manipulation via prompt injection |
| Plugins | Medium | Gated by marketplace trust, but still code |
| Settings, keybindings | Low | Model choice, shortcuts — no execution risk |

### Tiered behavior

**Low-risk surfaces** (settings, keybindings, CLAUDE.md, plugin list): applied automatically, shown in pull summary.

**High-risk surfaces** (permissions, MCP servers, hooks): require explicit user approval before applying.

### Interactive pull

```
$ claude-sync pull

  Pulling from origin...

  Safe changes (auto-applied):
    Settings: model → claude-opus-4-6
    CLAUDE.md: updated "Git Conventions" section
    Keybindings: no changes

  Requires approval:
    Permissions:
      + allow: Bash(kubectl *)
      + allow: Bash(docker *)
      + deny:  Bash(git push --force*)

    MCP servers:
      + linear (stdio: npx @linear/mcp)

    Hooks:
      + SessionStart: work-setup.sh

    Accept these changes? [y/N/review]
```

### Non-interactive pull (from hooks)

When `pull --auto` is invoked from a SessionStart hook:

1. Safe changes are applied silently
2. High-risk changes are written to `pending-changes.yaml` (gitignored)
3. Hook output reports pending changes
4. Claude Code sees the hook output and asks the user conversationally
5. User approves/rejects through Claude, which runs `claude-sync approve` or `claude-sync reject`

### Pending changes file

```yaml
# ~/.claude-sync/pending-changes.yaml (gitignored)
pending_since: "2026-02-13T10:30:00Z"
commit: "abc1234"
changes:
  permissions:
    add_allow: ["Bash(kubectl *)", "Bash(docker *)"]
    add_deny: ["Bash(git push --force*)"]
  mcp:
    add:
      linear:
        type: stdio
        command: npx
        args: [-y, "@linear/mcp"]
  hooks:
    add:
      SessionStart:
        - matcher: ""
          hooks:
            - type: command
              command: "work-setup.sh"
```

Pending changes persist across sessions. Claude mentions them on each startup until the user deals with them.

### Merge strategy

**Additive merge** for all surfaces. Synced config adds to local state, never removes locally-accumulated values. This allows organizations to push a base config while users retain local customizations.

## Auto-Sync: Incremental Tracking via Hooks

### Problem

When agents edit `~/.claude/CLAUDE.md` or `~/.claude/settings.json` during a session, those changes should flow back into the sync repo incrementally — small commits, not one big diff at the end.

### Hook design

Three hooks, registered by `claude-sync setup`:

| Hook Event | Command | Behavior |
|---|---|---|
| `PostToolUse` (Write/Edit) | `claude-sync auto-commit --if-changed` | Local commit only, no network |
| `SessionEnd` | `claude-sync push --auto --quiet` | Batch push accumulated commits to remote |
| `SessionStart` | `claude-sync pull --auto` | Push any unpushed commits first, then pull |

### PostToolUse: auto-commit

Triggered after any Write or Edit tool use. Checks if watched files changed:

- `~/.claude/CLAUDE.md` → run disassembly, update fragments, commit
- `~/.claude/settings.json` → extract syncable fields, update config.yaml, commit
- `~/.claude/.mcp.json` → update mcp section in config.yaml, commit

Commit messages are auto-generated: `"auto: update git-conventions fragment"`, `"auto: sync statusLine change"`.

No network calls. No push to remote. Fast and non-blocking.

### SessionEnd: batch push

Pushes all accumulated auto-commits to remote in one batch. Silent unless there's a conflict.

### SessionStart: push-first pull

Safety net for crashed sessions where SessionEnd never fired:

1. Check for unpushed local commits
2. Push them to remote first
3. Then pull (with tiered approval for high-risk changes)

### Example session flow

```
Session 1:
  Start  → pull --auto (nothing pending)
  Edit   → auto-commit "auto: update git-conventions fragment"
  Edit   → auto-commit "auto: sync statusLine change"
  Edit   → auto-commit "auto: add kubectl permission"
  End    → push 3 commits to remote

Session 2 (terminal killed mid-session):
  Start  → pull --auto
  Edit   → auto-commit "auto: update model setting"
  *crash* — SessionEnd never fires

Session 3:
  Start  → push 1 unpushed commit, then pull
  ...continues normally...
```

## New Commands

| Command | Description |
|---|---|
| `claude-sync approve` | Review and approve pending high-risk changes |
| `claude-sync reject` | Reject pending high-risk changes |
| `claude-sync auto-commit --if-changed` | Check watched files, commit if changed (non-interactive) |

Existing commands (`init`, `pull`, `push`, `status`, `profile`) are extended to handle new surfaces.

## Init Flow Changes

The init wizard adds steps for the new surfaces:

1. Config style selection (simple vs profiles) — existing
2. Plugin selection — existing
3. **CLAUDE.md import** — split existing CLAUDE.md into fragments, confirm naming
4. **Settings import** — select which settings to sync (model, statusLine, env)
5. **Permissions import** — select base allow/deny rules to sync
6. **MCP server import** — select which MCP servers to sync
7. **Keybindings import** — import keybindings.json if it exists
8. Hooks selection — existing
9. Profile creation (if profiles mode) — extended to cover all surfaces
10. Git init + commit + optional remote push — existing

## Pull Flow Changes

1. Push-first: push any unpushed local auto-commits
2. Git pull from remote
3. Parse config.yaml + active profile
4. **Assemble CLAUDE.md** from fragments and write to `~/.claude/CLAUDE.md`
5. Apply settings to `~/.claude/settings.json`
6. **Apply keybindings** to `~/.claude/keybindings.json`
7. Install/update plugins
8. **Tiered approval gate** for high-risk changes:
   - If interactive: prompt user
   - If non-interactive (`--auto`): write to `pending-changes.yaml`
9. Apply approved permissions to `~/.claude/settings.json`
10. Apply approved hooks to `~/.claude/settings.json`
11. **Apply approved MCP servers** to `~/.claude/.mcp.json`

## Push Flow Changes

1. Scan for changes across all surfaces:
   - Plugin changes (existing)
   - **CLAUDE.md disassembly** — split current file, match to fragments
   - **Settings diff** — compare syncable fields against config.yaml
   - **Permissions diff** — compare current allow/deny against config
   - **MCP diff** — compare current .mcp.json against config
   - **Keybindings diff** — compare keybindings.json against config
   - Hook changes (existing)
2. Show proposed changes, confirm with user
3. Update config.yaml, fragment files, profile files as needed
4. Commit and push to remote

## Repo Structure

```
~/.claude-sync/
├── config.yaml                # Main config (all surfaces)
├── claude-md/                 # CLAUDE.md fragments
│   ├── manifest.yaml          # Fragment metadata and ordering
│   ├── _preamble.md
│   ├── git-conventions.md
│   ├── color-preferences.md
│   └── subagent-workflow.md
├── profiles/                  # Profile overlays
│   ├── work.yaml
│   └── personal.yaml
├── plugins/                   # Forked plugin source code
│   └── custom-tool/
├── .gitignore
│
├── user-preferences.yaml      # (gitignored) Machine-local overrides
├── active-profile             # (gitignored) Current active profile name
├── pending-changes.yaml       # (gitignored) Unapproved high-risk changes
└── .last_fetch                # (gitignored) Last fetch timestamp
```

## Migration

Existing claude-sync repos (v1.0.0 with plugins/settings/hooks only) require no migration. New sections are optional and start empty until the user runs `init` or `push` to import them.

A `claude-sync migrate` command can assist with importing:
- Existing `~/.claude/CLAUDE.md` → fragment split
- Existing permissions from `~/.claude/settings.json` → permissions section
- Existing MCP config from `~/.claude/.mcp.json` → mcp section
- Existing keybindings → keybindings section

This is non-destructive — existing config fields are preserved, new fields are added alongside them.

## Implementation Phases

### Phase 1: Settings expansion + permissions
- Expand settings sync to cover statusLine, env vars
- Add permissions section to config and profile
- Add tiered approval on pull
- Add pending-changes.yaml flow

### Phase 2: CLAUDE.md fragment system
- Fragment storage, manifest, assembly/disassembly
- Init flow for splitting existing CLAUDE.md
- Push flow for reverse-mapping changes
- Profile add/remove for fragments

### Phase 3: MCP + keybindings
- MCP server sync with .mcp.json read/write
- Keybindings sync
- Profile support for both

### Phase 4: Auto-sync hooks
- PostToolUse auto-commit hook
- SessionEnd batch push hook
- SessionStart push-first pull
- `claude-sync setup` registers all hooks

### Phase 5: Approve/reject commands
- `claude-sync approve` interactive review
- `claude-sync reject` with reasons
- Claude Code conversational approval flow
