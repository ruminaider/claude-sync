# claude-sync Design Document

**Date:** 2026-01-28
**Status:** Draft
**Author:** Albert Gwo + Claude

## Overview

claude-sync is a tool for synchronizing Claude Code configuration across multiple machines. It provides declarative configuration management with automatic updates, profile-based customization, and full version history.

### Goals

1. **Personal multi-machine sync** - Same Claude Code setup everywhere
2. **Team/org distribution** - Share configurations with optional user preferences
3. **Plugin intelligence** - Track upstream updates, manage forks, selective sync
4. **Zero friction** - Auto-pull on startup, manual push with review

### Non-Goals

- Replacing Claude Code's plugin system
- Syncing conversation history (separate concern)
- Managing non-Claude-Code dotfiles

---

## Architecture

### Directory Structure

```
~/.claude-sync/                    # User's sync repo (Git-backed)
├── config.yaml                    # Declarative plugin/settings config
├── profiles/
│   ├── base.yaml                  # Common to all machines
│   ├── personal.yaml              # Personal machine additions
│   └── work.yaml                  # Work machine additions
├── project-mappings.yaml          # Project → profile mappings
├── plugins/                       # Forked/custom plugins
│   ├── compound-engineering-django-ts/
│   └── figma-minimal/
├── skills/                        # Custom skills
├── hooks/                         # Custom hooks
├── user-preferences.yaml          # Local-only overrides (not synced)
└── .git/                          # Full version history
```

### Config Schema

```yaml
# config.yaml
version: 2.4.1

plugins:
  # Track upstream - auto-updates
  upstream:
    - context7@claude-plugins-official
    - playwright@claude-plugins-official
    - episodic-memory@superpowers-marketplace

  # Manual updates only
  pinned:
    - beads@beads-marketplace: "0.44.0"

  # Synced from this repo
  forked:
    - compound-engineering-django-ts
    - figma-minimal

settings:
  model: opus
  # ... rest of settings

hooks:
  PreCompact: "bd prime"
  SessionStart: "bd prime"
```

### Profile Composition

```yaml
# profiles/base.yaml
plugins:
  upstream:
    - context7@claude-plugins-official
    - episodic-memory@superpowers-marketplace

settings:
  model: opus
```

```yaml
# profiles/personal.yaml
plugins:
  upstream:
    - frontend-design@claude-plugins-official
    - figma-minimal@figma-minimal-marketplace
```

```yaml
# profiles/work.yaml
plugins:
  upstream:
    - security-guidance@claude-plugins-official
    - pr-review-toolkit@claude-plugins-official
```

### Project Mapping

Projects are mapped to profiles using Git remote as stable identifier (survives folder moves/renames):

```yaml
# project-mappings.yaml
mappings:
  # Pattern rules (fallback)
  - match: "~/work/**"
    profile: work
  - match: "~/personal/**"
    profile: personal

  # Git remote rules (stable, preferred)
  - remote: "github.com/company/project"
    profile: work
  - remote: "github.com/username/side-project"
    profile: personal

default: personal
```

---

## Plugin Management

### Three Categories

| Category | Update Behavior | Source of Truth |
|----------|-----------------|-----------------|
| **Upstream** | Automatic | Marketplace |
| **Pinned** | Manual (notify only) | Marketplace, user-controlled |
| **Forked** | Automatic from repo | User's sync repo |

### Upstream Tracking

```bash
$ claude-sync status

UPSTREAM PLUGINS (auto-update)
  ✓ context7              e30768372b41  (latest)
  ⚡ episodic-memory       1.0.15 → 1.0.17 available

PINNED PLUGINS (manual)
  📌 beads                 0.44.0 (pinned) — 0.45.2 available

FORKED PLUGINS (from your repo)
  🔧 compound-engineering  2.22.0-custom
     ↳ upstream: 2.25.0 (+3 versions ahead)
```

### Forking Workflow

```bash
# Fork an upstream plugin for customization
$ claude-sync fork episodic-memory

# See upstream changes since forking
$ claude-sync upstream-diff compound-engineering

# Selectively apply upstream changes
$ claude-sync upstream-sync compound-engineering
```

### Selective Upstream Sync

Interactive selection of which upstream changes to apply:

```bash
$ claude-sync upstream-sync compound-engineering

Upstream changes available (2.22.0 → 2.25.0):

SKILLS (3 changes)
  [1] + code-reviewer (new skill)
  [2] ~ design-iterator (modified)
  [3] + changelog (new skill)

Select changes to apply (e.g., 1,3 or 'all' or 'none'): 1,3
```

---

## Versioning & Rollback

### Full-State Releases

Every sync state is versioned as a complete snapshot:

```bash
$ claude-sync releases

v2.4.1 (current)  2024-01-27  Added greptile, updated episodic-memory
v2.4.0            2024-01-20  Forked figma-minimal
v2.3.0            2024-01-15  Added work profile
```

### Rollback

```bash
# Rollback entire state
$ claude-sync rollback v2.3.0

Rolling back to v2.3.0...
Changes that will be reverted:
  - greptile (removed)
  - episodic-memory: 1.0.17 → 1.0.15

Apply rollback? [y/N]
```

Rollback creates a **new commit** (v2.4.2) with the state from v2.3.0 - history is never rewritten.

### Diff Between Versions

```bash
$ claude-sync diff v2.3.0 v2.4.1

ADDED:
  + greptile@claude-plugins-official

UPDATED:
  ~ episodic-memory: 1.0.15 → 1.0.17

SETTINGS CHANGED:
  ~ model: sonnet → opus
```

---

## Sync Workflow

### Automation Model

| Action | How it happens |
|--------|----------------|
| Pulling config changes | Automatic on Claude Code startup |
| Applying upstream plugin updates | Automatic (unless pinned) |
| Pushing your changes | Manual via CLI/TUI |
| Changing profiles | Manual via CLI/TUI |

**Philosophy: Reads are automatic, writes require intent.**

### Conflict Avoidance

Conflicts are prevented through process:

1. **Auto-pull on startup** - Always start from latest state
2. **Pull before push** - `push` automatically pulls first
3. **Fast-forward only** - Rejects push if behind, prompts to pull
4. **Non-overlapping changes** - Most edits don't conflict

```bash
$ claude-sync push

Checking for remote changes...
⚠️  Remote has 2 new commits. Pulling first...
✓ Pulled latest changes
✓ Your changes applied cleanly on top
✓ Pushed successfully (v2.4.2)
```

### Disagree and Override

If you disagree with a pulled change:

```bash
# You're not "behind" - you're creating a corrective change
$ claude-sync set model opus
$ claude-sync push -m "Reverting to opus - sonnet caused issues"

✓ Pushed v2.4.2
```

---

## User Preferences

### Layer Model

```
┌─────────────────────────────────────────┐
│  User Preferences (highest priority)    │  ← Personal overrides (local only)
├─────────────────────────────────────────┤
│  Org/Team Config                        │  ← Shared baseline (synced)
├─────────────────────────────────────────┤
│  Profile (base, work, personal)         │  ← Composable defaults
└─────────────────────────────────────────┘
```

### User Preferences File

```yaml
# ~/.claude-sync/user-preferences.yaml (local only, never synced)

settings:
  model: opus  # Override org default

plugins:
  unsubscribe:
    - ralph-wiggum      # Opt out
    - greptile          # Don't have API key
  personal:
    - some-niche-plugin # Add personal plugin

pins:
  episodic-memory: "1.0.15"  # Pin even if org tracks upstream

sync:
  auto_update_plugins: false
  notify_on_update: terminal
```

### Subscribe/Unsubscribe Flow

```bash
$ claude-sync pull

Org config updated (v2.5.0):
  + Added: new-linting-plugin

  [y] Accept
  [n] Unsubscribe (won't install, won't ask again)
  [l] Later (skip for now)

Choice: n
✓ Unsubscribed from new-linting-plugin
```

---

## CLI Interface

### Core Commands

```bash
claude-sync                    # Interactive mode / status
claude-sync init               # New setup from current state
claude-sync join <url>         # Join existing config
claude-sync status             # Show current state
claude-sync pull               # Pull latest from remote
claude-sync push [-m "msg"]    # Push changes with message
claude-sync update             # Apply available plugin updates
```

### Plugin Commands

```bash
claude-sync fork <plugin>              # Fork upstream for customization
claude-sync unfork <plugin>            # Return to upstream
claude-sync pin <plugin> [version]     # Pin to specific version
claude-sync unpin <plugin>             # Enable auto-updates
claude-sync upstream-diff <plugin>     # See upstream changes
claude-sync upstream-sync <plugin>     # Selectively apply upstream
```

### Version Commands

```bash
claude-sync releases                   # List all versions
claude-sync release <version> -m "msg" # Create release
claude-sync rollback [version]         # Rollback to version
claude-sync diff <v1> <v2>             # Compare versions
claude-sync log                        # Show history
```

### Profile Commands

```bash
claude-sync profile list               # List available profiles
claude-sync profile set <name>         # Set profile for current project
claude-sync profile show               # Show active profile
```

### Preference Commands

```bash
claude-sync preferences                # Interactive preferences UI
claude-sync subscribe <plugin>         # Re-enable org plugin
claude-sync unsubscribe <plugin>       # Opt out of org plugin
```

---

## First-Run Experience

### New vs Join

```bash
$ claude-sync

Welcome to claude-sync!

  [1] New setup - create config from current Claude Code state
  [2] Join existing - connect to a config repo

Choice:
```

### New Setup (init)

```bash
Choice: 1

Scanning your current Claude Code setup...
  ✓ Found 24 plugins
  ✓ Found 3 custom hooks

Detected plugins:
  • 20 from official marketplace → tracking as 'upstream'
  • 2 local/custom → tracking as 'forked'

Does this look right? [Y/n]
```

### Join Existing

```bash
Choice: 2

Enter config repo URL:
> git@github.com:ruminaider/claude-sync-config.git

This config includes:
  • 22 plugins (18 upstream, 2 pinned, 2 forked)
  • 2 profiles: base, personal

  [a] Apply - replace my setup
  [m] Merge - keep extras, add missing
  [p] Preview - show diff first
```

---

## Claude Code Integration

### As a Plugin

```bash
# Add marketplace
claude plugins marketplace add claude-sync-marketplace \
  --source github:ruminaider/claude-sync-marketplace

# Install
claude plugins install claude-sync@claude-sync-marketplace
```

### Slash Commands

```bash
/sync status
/sync pull
/sync push
/sync update
```

### MCP Server

```yaml
resources:
  - uri: claude-sync://status
  - uri: claude-sync://plugins
  - uri: claude-sync://history
```

### Startup Hook

```yaml
# In PLUGIN.md
hooks:
  SessionStart: claude-sync pull --quiet
```

---

## Implementation

### Tech Stack

| Component | Choice | Rationale |
|-----------|--------|-----------|
| Core | Go | Fast startup, single binary, excellent concurrency |
| Config | YAML | Human-readable, good tooling |
| Storage | Git | Version history, proven reliability |
| TUI | Bubble Tea (Go) | Modern terminal UI library |

### Target Performance

| Operation | Target |
|-----------|--------|
| `pull` (no changes) | < 200ms |
| `pull` (with changes) | < 1s |
| `status` | < 500ms |
| Startup hook | Non-blocking |

### Project Structure

```
claude-sync/
├── cmd/claude-sync/          # CLI entrypoint
├── internal/
│   ├── config/               # YAML parsing, merging
│   ├── plugins/              # Plugin management
│   ├── profiles/             # Profile composition
│   ├── sync/                 # Git operations
│   ├── marketplace/          # Marketplace API client
│   └── tui/                  # Terminal UI
├── docs/
│   └── plans/                # Design documents
└── scripts/                  # Build/release scripts
```

### Phases

**Phase 1: Core sync (MVP)**
- Git-based config repo
- Pull/push commands
- Basic settings.json generation
- Auto-pull on startup

**Phase 2: Plugin intelligence**
- Upstream/pinned/forked categorization
- Marketplace update checking
- Plugin version tracking

**Phase 3: Profiles**
- Profile composition
- Project mapping with git remote detection
- Per-machine config

**Phase 4: Polish**
- Interactive TUI
- Selective upstream sync for forks
- Release management
- User preferences layer

---

## Distribution

### Initial Release

- **GitHub releases** - Binary downloads for macOS/Linux
- **Homebrew tap** - `brew install ruminaider/tap/claude-sync`
- **Claude Code plugin marketplace** - `claude-sync-marketplace`

### Future

- Potential adoption into `claude-plugins-official` if widely used

---

## Open Questions

1. **Marketplace API** - How do we check for plugin updates? Need to understand Claude Code's marketplace API or scrape from known sources.

2. **Settings.json generation** - What's the exact merge behavior when combining profiles? Need to define precedence rules.

3. **Plugin installation** - Should we call Claude Code CLI or directly manipulate `~/.claude/plugins/`?

4. **Cross-platform** - Windows support? WSL only initially?

---

## Appendix: Full CLI Reference

```
claude-sync - Sync Claude Code configuration across machines

USAGE:
    claude-sync [command]

COMMANDS:
    init                Create new config from current setup
    join <url>          Join existing config repo

    status              Show sync status
    pull                Pull latest config
    push                Push changes to remote

    update              Apply available plugin updates
    fork <plugin>       Fork plugin for customization
    unfork <plugin>     Return to upstream
    pin <plugin>        Pin to specific version
    unpin <plugin>      Enable auto-updates

    upstream-diff       Show upstream changes for fork
    upstream-sync       Selectively apply upstream changes

    releases            List all versions
    release <version>   Create new release
    rollback [version]  Rollback to previous state
    diff <v1> <v2>      Compare versions
    log                 Show history

    profile list        List profiles
    profile set <name>  Set profile for project
    profile show        Show active profile

    preferences         Interactive preferences UI
    subscribe           Re-enable org plugin
    unsubscribe         Opt out of org plugin

    configure           Configure sync settings
    help                Show help

FLAGS:
    -q, --quiet         Suppress output
    -v, --verbose       Verbose output
    --dry-run           Preview without applying
```
