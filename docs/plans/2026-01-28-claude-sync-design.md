# claude-sync Design Document

**Date:** 2026-01-28
**Revised:** 2026-01-29
**Status:** Ready for Implementation
**Author:** Albert Gwo + Claude

## Overview

claude-sync is a tool for synchronizing Claude Code configuration across multiple machines. It provides declarative configuration management with automatic updates, profile-based customization, and full version history.

### Goals

1. **Personal multi-machine sync** - Same Claude Code setup everywhere
2. **Team/org distribution** - Share configurations with optional user preferences
3. **Plugin intelligence** - Track upstream updates, manage forks, selective sync
4. **Zero friction** - Auto-pull on startup, notify user of pending updates

### Non-Goals

- Replacing Claude Code's plugin system
- Syncing conversation history (separate concern)
- Managing non-Claude-Code dotfiles
- Hot-reloading plugins mid-session (not supported by Claude Code)

---

## Key Design Decisions

Decisions made during design review (2026-01-29):

| Topic | Decision |
|-------|----------|
| **Plugin sync model** | Hybrid: metadata for upstream/pinned, full files for forked |
| **Forked plugin storage** | `~/.claude-sync/plugins/` exposed as local marketplace |
| **Update notification** | Notify + restart pattern via SessionStart hook |
| **Update failures** | Warn and continue with cached version |
| **Config merge** | Implicit (scalars override, arrays union) + `$remove`/`$replace` escape hatches |
| **Marketplace API** | Git-based (no REST API) - query marketplace repos directly |
| **Plugin installation** | Use `claude plugin install` CLI |
| **Cross-platform** | macOS/Linux first, Windows/WSL deferred |

---

## Claude Code Integration Points

### File Structure (from official docs + local research)

```
~/.claude/                          # Claude Code user directory
├── settings.json                   # Global settings (model, permissions, hooks)
├── settings.local.json             # Local overrides (gitignored)
├── CLAUDE.md                       # Global instructions
├── plugins/
│   ├── installed_plugins.json      # Plugin versions, paths, timestamps (v2 format)
│   ├── known_marketplaces.json     # Marketplace sources (github/git/directory)
│   └── cache/                      # Downloaded plugin files (NOT synced)
│       └── <marketplace>/<plugin>/<version>/
├── skills/                         # User skills
├── commands/                       # Custom slash commands
└── agents/                         # Custom agents
```

### What claude-sync Syncs

| File | Synced? | Notes |
|------|---------|-------|
| `settings.json` | ✅ Yes | Model, hooks, basic settings |
| `installed_plugins.json` | ✅ Yes | Plugin list (metadata only) |
| `known_marketplaces.json` | ✅ Yes | Marketplace sources |
| `plugins/cache/` | ❌ No | Re-downloaded from marketplace |
| Forked plugins | ✅ Yes | Stored in `~/.claude-sync/plugins/` |

### Implementation Notes: Claude Code File Formats

**`installed_plugins.json` (v2 format):**
- Top-level `version: 2` field
- Plugins keyed as `{name}@{marketplace-id}` → **array** of installations (not a single entry)
- Each entry has: `scope`, `installPath`, `version`, `installedAt`, `lastUpdated`, optional `gitCommitSha`

**`known_marketplaces.json` source types:**
- `github` — uses `repo` field (`"owner/repo"`)
- `git` — uses `url` field (for self-hosted repos)
- `directory` — uses `path` field (for local plugins, used by claude-sync forks)
- Each entry also has `installLocation` and `lastUpdated`

**Version string formats (mixed across plugins):**
- Semantic versions: `1.0.0`, `0.44.0`
- Git commit short SHAs: `e30768372b41`
- Custom tags: `2.22.0-custom`
- Version comparison logic must handle all three — SHAs cannot be compared with `>`

### Settings Precedence (Claude Code native)

```
1. Managed settings (enterprise) - cannot override
2. Command-line args - temporary
3. .claude/settings.local.json - local project
4. .claude/settings.json - shared project
5. ~/.claude/settings.json - user global ← claude-sync targets this
```

### Plugin Hot-Reload Limitation

Claude Code **does not support plugin hot-reload** (Issue #18174). Plugins are loaded at session startup. Therefore:

- Updates applied by claude-sync require session restart
- SessionStart hook notifies user of pending updates
- User decides when to restart

> **Note:** The `claude plugin install/uninstall` CLI commands work fine while Claude Code sessions are running — they modify `installed_plugins.json` but don't affect active sessions. Changes take effect on next session start.

---

## Architecture

### Directory Structure

```
~/.claude-sync/                    # User's sync repo (Git-backed)
├── config.yaml                    # Declarative plugin/settings config
├── profiles/                      # Phase 3
│   ├── base.yaml                  # Common to all machines
│   ├── personal.yaml              # Personal machine additions
│   └── work.yaml                  # Work machine additions
├── project-mappings.yaml          # Project → profile mappings (Phase 3)
├── plugins/                       # Forked/custom plugins (Phase 2)
│   ├── compound-engineering-django-ts/
│   └── figma-minimal/
├── user-preferences.yaml          # Local-only overrides (not synced)
└── .git/                          # Full version history
```

### Config Schema

#### Phase 1 (MVP) - Simple Format

```yaml
# config.yaml
version: 1.0.0

plugins:
  - context7@claude-plugins-official
  - playwright@claude-plugins-official
  - episodic-memory@superpowers-marketplace
  - beads@beads-marketplace

settings:
  model: opus

hooks:
  PreCompact: "bd prime"
  SessionStart: "claude-sync pull --quiet"
```

#### Phase 2+ - Categorized Format

```yaml
# config.yaml
version: 2.0.0

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

hooks:
  PreCompact: "bd prime"
  SessionStart: "claude-sync pull --quiet"
```

---

## Plugin Management

### Sync Model (Hybrid Approach)

| Plugin Type | What's Synced | On Pull |
|-------------|---------------|---------|
| **Upstream** | Name only (`context7@marketplace`) | Reinstall from marketplace |
| **Pinned** | Name + version (`beads@marketplace: "0.44.0"`) | Reinstall specific version |
| **Forked** | Full plugin files in `~/.claude-sync/plugins/` | Copy/link to Claude Code |

**Rationale:**
- Upstream/pinned plugins are small metadata (~1KB) - actual files re-downloaded
- Forked plugins ARE synced because they don't exist in any marketplace
- Sync repo stays small (just forks + config)

### Forked Plugin Mechanism

Forked plugins are exposed to Claude Code as a **local marketplace**:

```json
// Added to ~/.claude/plugins/known_marketplaces.json
{
  "claude-sync-forks": {
    "source": {
      "source": "directory",
      "path": "/Users/<user>/.claude-sync/plugins"
    },
    "installLocation": "/Users/<user>/.claude/plugins/marketplaces/claude-sync-forks",
    "lastUpdated": "2026-01-29T00:00:00.000Z"
  }
}
```

> **Note:** The actual format uses `path` (not `directory`) and requires an absolute path. The `installLocation` and `lastUpdated` fields are also required.

Plugins then appear as `compound-engineering-django-ts@claude-sync-forks`.

### Three Categories (Phase 2+)

| Category | Update Behavior | Source of Truth |
|----------|-----------------|-----------------|
| **Upstream** | Automatic | Marketplace |
| **Pinned** | Manual (notify only) | Marketplace, user-controlled |
| **Forked** | Automatic from repo | User's sync repo |

### Update Notification Behavior

| Scenario | Behavior |
|----------|----------|
| Upstream plugin has update | Check on SessionStart, notify via `additionalContext` |
| Pinned plugin has update | Terminal message: `📌 beads 0.44.0 pinned — 0.45.2 available` |
| Update fails (network, etc.) | Warn in terminal, continue with cached version |
| Forked plugin changed in repo | Sync automatically from git |

### Status Display

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

---

## Config Merge Semantics

### Default Behavior (Implicit)

| Field Type | Merge Behavior | Example |
|------------|----------------|---------|
| **Scalar** (string, number, bool) | Last wins | `model: opus` + `model: sonnet` → `sonnet` |
| **Object** | Deep merge recursively | `settings.permissions` merges |
| **Array** | Union, deduplicated | `[a, b]` + `[b, c]` → `[a, b, c]` |

### Profile Load Order (Phase 3)

```
1. profiles/base.yaml        ← foundation
2. profiles/<active>.yaml    ← machine-specific (personal/work)
3. user-preferences.yaml     ← local overrides (never synced)
```

Each layer merges on top of the previous.

### Escape Hatches (Explicit Operators)

For cases where implicit merge isn't sufficient:

| Operator | Behavior |
|----------|----------|
| `$replace: [...]` | Replace entire array, don't merge |
| `$remove: [...]` | Remove items from merged result |

### Example with Escape Hatches

```yaml
# profiles/work.yaml
settings:
  model: sonnet              # Overrides base.yaml's "opus"

plugins:
  upstream:
    - security-guidance      # Added to base plugins (union)
    - greptile
    $remove:
      - episodic-memory      # Explicitly removed from base
```

---

## Sync Workflow

### Update Strategy: Notify + Restart

Since Claude Code doesn't support plugin hot-reload, claude-sync uses a **notify + restart** pattern:

```
┌─────────────────────────────────────────────────────────────┐
│                    SessionStart Hook                         │
└─────────────────────────────────────────────────────────────┘
                            │
                            ▼
              ┌───────────────────────────┐
              │  claude-sync pull --quiet │
              │  (updates local config)   │
              └───────────────────────────┘
                            │
                            ▼
              ┌───────────────────────────┐
              │  Compare: installed vs    │
              │  config desired state     │
              └───────────────────────────┘
                            │
              ┌─────────────┴─────────────┐
              │                           │
              ▼                           ▼
        No changes                  Updates needed
              │                           │
              ▼                           ▼
        Return {}              Return additionalContext:
                               "⚠️ 3 plugin updates pending.
                                Restart session to apply."
```

### Hook Output Format

```json
{
  "hookSpecificOutput": {
    "hookEventName": "SessionStart",
    "additionalContext": "claude-sync: 3 plugin update(s) available. Run '/sync status' for details. Restart session to apply updates."
  }
}
```

Claude receives this context and naturally informs the user.

### User Experience

```
$ claude

Claude: I notice there are 3 plugin updates available from your
claude-sync configuration. To apply them, you'll need to restart
this session. Would you like me to help you wrap up, or continue
with the current versions?

> continue for now

Claude: No problem, continuing with current versions...
```

### Automation Model

| Action | How it happens |
|--------|----------------|
| Pulling config changes | Automatic on Claude Code startup (SessionStart hook) |
| Checking for plugin updates | Automatic on startup, compare installed vs desired |
| Notifying user | Via `additionalContext` in hook output |
| Applying updates | Requires session restart (Claude Code limitation) |
| Pushing your changes | Manual via CLI (`claude-sync push`) |

**Philosophy: Reads are automatic, writes require intent, updates require restart.**

### Conflict Avoidance

1. **Auto-pull on startup** - Always start from latest config state
2. **Pull before push** - `push` automatically pulls first
3. **Fast-forward only** - Rejects push if behind, prompts to pull
4. **Non-overlapping changes** - Most edits don't conflict

---

## User Preferences

### Layer Model

```
┌─────────────────────────────────────────┐
│  User Preferences (highest priority)    │  ← Personal overrides (local only)
├─────────────────────────────────────────┤
│  Org/Team Config                        │  ← Shared baseline (synced)
├─────────────────────────────────────────┤
│  Profile (base, work, personal)         │  ← Composable defaults (Phase 3)
└─────────────────────────────────────────┘
```

### User Preferences File

```yaml
# ~/.claude-sync/user-preferences.yaml (local only, never synced)

sync_mode: union  # Options: union (default), exact
                  # union: Add missing plugins, keep local extras
                  # exact: Match config exactly (remove extras not in config)

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
```

---

## CLI Interface

### Phase 1 Commands (MVP)

```bash
claude-sync                    # Interactive mode / status
claude-sync init               # New setup from current state
claude-sync join <url>         # Join existing config
claude-sync status             # Show current state
claude-sync pull               # Pull latest from remote
claude-sync push [-m "msg"]    # Push changes with message
```

### Phase 2 Commands (Plugin Intelligence)

```bash
claude-sync update             # Apply available plugin updates
claude-sync fork <plugin>      # Fork upstream for customization
claude-sync unfork <plugin>    # Return to upstream
claude-sync pin <plugin> [ver] # Pin to specific version
claude-sync unpin <plugin>     # Enable auto-updates
```

### Phase 3 Commands (Profiles)

```bash
claude-sync profile list       # List available profiles
claude-sync profile set <name> # Set profile for current project
claude-sync profile show       # Show active profile
```

### Phase 4 Commands (Polish)

```bash
claude-sync releases           # List all versions
claude-sync release <ver>      # Create release
claude-sync rollback [version] # Rollback to version
claude-sync diff <v1> <v2>     # Compare versions
claude-sync upstream-diff      # Show upstream changes for fork
claude-sync upstream-sync      # Selectively apply upstream
claude-sync daemon start|stop  # Background sync daemon
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
  • context7@claude-plugins-official
  • episodic-memory@superpowers-marketplace
  • beads@beads-marketplace
  ... (21 more)

Create sync repo at ~/.claude-sync? [Y/n]

✓ Created ~/.claude-sync/
✓ Generated config.yaml
✓ Initialized git repository

Next steps:
  1. Review config: cat ~/.claude-sync/config.yaml
  2. Add remote: cd ~/.claude-sync && git remote add origin <url>
  3. Push: claude-sync push -m "Initial config"
```

---

## Claude Code Plugin Integration

### As a Plugin

```bash
# Add marketplace
claude plugin marketplace add claude-sync-marketplace \
  --source github:ruminaider/claude-sync-marketplace

# Install
claude plugin install claude-sync@claude-sync-marketplace
```

### Slash Commands (via plugin)

```bash
/sync status    # Show sync status
/sync pull      # Pull latest config
/sync push      # Push changes
/sync apply     # Install pending updates (Phase 2)
```

### Startup Hook (in plugin's hooks.json)

```json
{
  "hooks": {
    "SessionStart": [
      {
        "hooks": [
          {
            "type": "command",
            "command": "${CLAUDE_PLUGIN_ROOT}/hooks/session-start.sh",
            "timeout": 10
          }
        ]
      }
    ]
  }
}
```

---

## Implementation

### Tech Stack

| Component | Choice | Rationale |
|-----------|--------|-----------|
| Core | Go | Fast startup, single binary, excellent concurrency |
| Config | YAML | Human-readable, good tooling |
| Storage | Git | Version history, proven reliability |
| TUI | Bubble Tea (Go) | Modern terminal UI library (Phase 1 for push, expanded Phase 4) |

### Target Performance

| Operation | Target |
|-----------|--------|
| `pull` (no changes) | < 200ms |
| `pull` (with changes) | < 1s |
| `status` | < 500ms |
| SessionStart hook | < 500ms (non-blocking notification) |

### Project Structure

```
claude-sync/
├── cmd/claude-sync/          # CLI entrypoint
├── internal/
│   ├── config/               # YAML parsing, merging
│   ├── plugins/              # Plugin management
│   ├── profiles/             # Profile composition (Phase 3)
│   ├── sync/                 # Git operations
│   ├── marketplace/          # Marketplace client (Phase 2)
│   ├── hooks/                # Hook scripts
│   └── tui/                  # Terminal UI (Phase 1: push selector, Phase 4: full TUI)
├── plugin/                   # Claude Code plugin package
│   ├── .claude-plugin/
│   │   └── plugin.json
│   ├── bin/                  # Bundled binaries (built by CI)
│   │   ├── claude-sync-darwin-arm64
│   │   ├── claude-sync-darwin-amd64
│   │   ├── claude-sync-linux-arm64
│   │   └── claude-sync-linux-amd64
│   ├── hooks/
│   │   └── session-start.sh
│   ├── commands/
│   │   └── sync.md
│   └── README.md
├── docs/
│   └── plans/
└── scripts/
```

### Phases

**Phase 1: Minimal Viable Sync (MVP)**
- `init`, `join`, `pull`, `push`, `status` commands
- Simple plugin list (no categories)
- Sync `settings.json` and plugin list
- SessionStart hook with update notification
- Git-based config repo
- Interactive TUI for `push` (Bubble Tea checkbox selector)
- `sync_mode` support (union/exact) in user-preferences.yaml

**Phase 2: Plugin Intelligence**
- Upstream/pinned/forked categorization
- Marketplace version checking (query git repos)
- `/sync apply` command
- Forked plugin storage in `~/.claude-sync/plugins/`
- Local marketplace registration

**Phase 3: Profiles**
- Profile composition (`base.yaml` + `personal.yaml`)
- Config merge semantics with escape hatches
- Project mapping via git remote
- `user-preferences.yaml` layer

**Phase 4: Polish**
- Full interactive TUI (Bubble Tea — beyond push selector)
- `claude-sync daemon` for background sync
- Selective upstream sync for forks
- Release management and rollback
- Full CLI reference

---

## Command Specifications (Phase 1)

Detailed behavior for each MVP command. These specifications are implementation-ready.

### `init` — Scan Current State

**Purpose:** Create `~/.claude-sync/config.yaml` from the user's current Claude Code installation.

**Algorithm:**

```
1. Read ~/.claude/plugins/installed_plugins.json
2. For each plugin in plugins[]:
   - Extract plugin name and marketplace from key (e.g., "beads@beads-marketplace")
   - Add to config.yaml plugins list
3. Read ~/.claude/settings.json
4. Extract ONLY these fields for config.yaml:
   - model (if present)
   - hooks (filtered — see below)
5. Initialize git repo in ~/.claude-sync/
```

**Hook Filtering Logic:**

Plugins can contribute hooks via their own `hooks.json`. To avoid duplicating plugin hooks into `config.yaml`, `init` must filter them out:

```
1. Build set of plugin hook commands:
   For each installed plugin:
     Read <plugin-cache-path>/hooks/hooks.json (if exists)
     Add all hook commands to known_plugin_hooks set
2. For each hook in settings.json:
   If hook.command NOT IN known_plugin_hooks:
     Include in config.yaml (it's user-defined)
```

**Fields NOT synced:**

- `enabledPlugins` — this is per-machine state (user may want different plugins enabled on different machines)
- `statusLine` — often machine-specific
- `permissions` — security-sensitive, should stay local

**Output:** `~/.claude-sync/config.yaml` matching the Phase 1 schema.

---

### `pull` — Apply Remote Config Locally

**Purpose:** Pull git changes and reconcile local Claude Code state with `config.yaml`.

**Algorithm:**

```
1. cd ~/.claude-sync && git pull --ff-only
   - If conflicts: abort and warn user to resolve manually
2. Read user-preferences.yaml → get sync_mode (default: union)
3. Parse config.yaml → desired state
4. Read installed_plugins.json → current state
5. Register claude-sync-forks marketplace (if forked plugins exist):
   - Check if config.yaml contains forked plugins (Phase 2+) or ~/.claude-sync/plugins/ is non-empty
   - If yes: update ~/.claude/plugins/known_marketplaces.json to include claude-sync-forks entry
   - This MUST happen before step 7 (plugin installation) or forked plugins will fail to install
6. Compute diff:
   - to_install: plugins in desired but not current
   - to_remove: plugins in current but not desired
   - to_update: plugins where version differs (Phase 2)
   - local_only: plugins in current but not in desired (extras)
7. Handle sync_mode:
   - If sync_mode == "union":
     - Skip to_remove (keep local extras)
     - If local_only is non-empty: show informational note (see below)
   - If sync_mode == "exact":
     - Show warning about plugins to be removed
     - Require confirmation before proceeding
8. For each plugin in to_install:
   - Run: claude plugin install <name>@<marketplace>
   - This command works non-interactively
9. For each plugin in to_remove (only if sync_mode == "exact" and confirmed):
   - Run: claude plugin uninstall <name>@<marketplace>
10. Merge settings:
    - Read current settings.json
    - Deep merge config.yaml settings ON TOP OF current settings
    - Preserve: enabledPlugins, statusLine, permissions, plugin-contributed hooks
    - Write back to settings.json
```

**Output Examples:**

Clean pull (no merge conflicts, union mode):
```
Pulled latest config.
  ✓ 2 plugins installed
  ✓ Settings updated

Note: 3 plugins installed locally but not in config:
  • my-local-plugin@some-marketplace
  • another-plugin@another-marketplace
  • experimental@local
Run 'claude-sync push' to add them, or keep as local-only.
```

Pull with removals (config removes plugins you have, union mode):
```
⚠️  Config changes would remove plugins you have installed:
  • old-plugin@marketplace
  • deprecated-tool@marketplace

Your sync_mode is 'union' — these will be KEPT locally.
To remove them: run 'claude-sync pull --exact' or set sync_mode: exact in user-preferences.yaml

Proceeding with union merge...
  ✓ 2 plugins installed
  ✓ Settings updated
```

**Important:** `pull` does NOT do a full overwrite of `settings.json`. It merges only the fields defined in `config.yaml`, preserving local-only fields.

**Exit codes:**
- 0: Success (changes applied or no changes needed)
- 1: Git pull failed (conflicts, network error)
- 2: Plugin installation failed (network error)

---

### `push` — Capture Local State to Remote

**Purpose:** Update `config.yaml` with current local state and push to git.

**Algorithm:**

```
1. Run `pull` first (fast-forward to latest)
   - If pull fails: abort
2. Scan current state (same as init):
   - Read installed_plugins.json
   - Read settings.json (filtered fields only)
3. Diff against existing config.yaml:
   - added_plugins: in current but not in config
   - removed_plugins: in config but not in current
   - changed_settings: settings that differ
4. If no changes: exit with "Nothing to push"
5. If changes found:
   - Show interactive TUI for selection (see below)
   - Update config.yaml with selected changes
   - git add config.yaml
   - git commit -m "<message or auto-generated>"
   - git push
```

**Interactive Selection (Bubble Tea TUI):**

```
$ claude-sync push

Scanning local state...

Plugins not in config:
  [x] my-local-plugin@some-marketplace
  [x] another-plugin@another-marketplace
  [ ] experimental@local              (unselected — keep local-only)

Settings changes:
  [x] model: opus → sonnet

Use arrow keys to navigate, space to toggle, enter to confirm.

> Confirm push? [Y/n]
```

- All items selected by default (user deselects what they want to keep local)
- Arrow keys navigate, space toggles, enter confirms
- Unselected plugins remain installed locally but are NOT added to config
- If `-m "message"` flag provided, skip confirmation prompt after selection

**Non-interactive mode:**

For scripting/CI, support `--all` and `--none` flags:
- `claude-sync push --all -m "sync all"` — push everything without TUI
- `claude-sync push --none` — dry-run, show what would be pushed

**Auto-generated commit messages:**

```
"Add context7, episodic-memory; remove beads"
"Update model to opus"
"Add playwright; update hooks"
```

---

### `status` — Show Current Sync State

**Purpose:** Display comparison between config.yaml (desired) and installed_plugins.json (actual).

**Algorithm:**

```
1. Parse config.yaml → desired
2. Parse installed_plugins.json → actual
3. For each plugin in desired:
   - Check if in actual → ✓ synced
   - Check if missing → ⚠️ not installed
4. For each plugin in actual but not desired:
   - Show as "untracked" (installed locally but not in config)
5. Compare settings.json fields against config.yaml
   - Show diffs for model, hooks
```

**Output format:**

```
SYNCED
  ✓ context7@claude-plugins-official
  ✓ episodic-memory@superpowers-marketplace

NOT INSTALLED (run 'claude-sync pull' to install)
  ⚠️ beads@beads-marketplace

UNTRACKED (run 'claude-sync push' to add to config)
  ? some-local-plugin@local-marketplace

SETTINGS
  model: opus (synced)
  hooks: 2 synced, 1 local-only
```

---

### `join` — Connect to Existing Config Repo

**Purpose:** Clone an existing config repo and apply it locally.

**Algorithm:**

```
1. If ~/.claude-sync/ exists:
   - Error: "Already initialized. Run 'claude-sync pull' instead."
2. git clone <url> ~/.claude-sync/
3. Run `pull` to apply config
```

---

### SessionStart Hook — Update Notification

**Purpose:** On Claude Code startup, check for pending updates and notify user via hook output.

**Location:** `plugin/hooks/session-start.sh` (shell script bundled with claude-sync plugin)

**Algorithm:**

```bash
#!/bin/bash
set -e

# Determine platform and select bundled binary
SCRIPT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
ARCH=$(uname -m)
OS=$(uname -s | tr '[:upper:]' '[:lower:]')

case "$ARCH" in
  x86_64) ARCH="amd64" ;;
  aarch64|arm64) ARCH="arm64" ;;
esac

CLAUDE_SYNC="${SCRIPT_DIR}/bin/claude-sync-${OS}-${ARCH}"

# Fallback: if bundled binary doesn't exist, try PATH
if [ ! -x "$CLAUDE_SYNC" ]; then
  CLAUDE_SYNC="claude-sync"
fi

# 1. Fast git fetch with timestamp-based skip (avoid redundant fetches)
cd ~/.claude-sync 2>/dev/null || { echo "{}"; exit 0; }

LAST_FETCH_FILE=~/.claude-sync/.last_fetch
FETCH_INTERVAL=30  # seconds

now=$(date +%s)
last_fetch=$(cat "$LAST_FETCH_FILE" 2>/dev/null || echo 0)

if [ $((now - last_fetch)) -gt $FETCH_INTERVAL ]; then
  git fetch --quiet 2>/dev/null || true
  echo "$now" > "$LAST_FETCH_FILE"
fi

# 2. Check if behind remote
LOCAL=$(git rev-parse HEAD)
REMOTE=$(git rev-parse @{u} 2>/dev/null || echo "$LOCAL")

if [ "$LOCAL" != "$REMOTE" ]; then
  CONFIG_CHANGES=true
else
  CONFIG_CHANGES=false
fi

# 3. Compare installed vs desired (quick check)
# Uses bundled binary for actual comparison
PLUGIN_DIFF=$("$CLAUDE_SYNC" diff --quiet 2>/dev/null || echo "")

# 4. Build notification message
if [ "$CONFIG_CHANGES" = true ] || [ -n "$PLUGIN_DIFF" ]; then
  MSG="claude-sync: Updates available."
  if [ "$CONFIG_CHANGES" = true ]; then
    MSG="$MSG Config changes pending."
  fi
  if [ -n "$PLUGIN_DIFF" ]; then
    MSG="$MSG $PLUGIN_DIFF"
  fi
  MSG="$MSG Run '/sync status' for details. Restart session to apply."

  # Output hook JSON
  cat <<EOF
{
  "hookSpecificOutput": {
    "hookEventName": "SessionStart",
    "additionalContext": "$MSG"
  }
}
EOF
else
  # No updates — output empty JSON (no notification)
  echo "{}"
fi
```

**Binary Resolution:**
1. Hook script detects platform (darwin/linux, arm64/amd64)
2. Uses bundled binary at `${CLAUDE_PLUGIN_ROOT}/bin/claude-sync-<os>-<arch>`
3. Falls back to `claude-sync` in PATH if bundled binary missing (for development)

**Performance requirement:** Must complete in <500ms. The `git fetch` is the slowest part — use `--quiet` and timeout after 2s if network is slow.

**Fallback:** If any step fails (network, git, binary not found, ~/.claude-sync doesn't exist), output `{}` and continue silently. Never block Claude Code startup.

---

## Error Handling

### Edge Case 1: `~/.claude-sync/` Exists But Is Not a Git Repo

**Scenario:** User accidentally deletes `.git/` or creates the directory manually.

**Behavior:** Error out with clear, actionable message:

```
Error: ~/.claude-sync is not a valid git repository

The sync directory exists but appears to be corrupted or incomplete.

To resolve:
  Option 1 - Rejoin your remote config:
    rm -rf ~/.claude-sync
    claude-sync join <your-remote-url>

  Option 2 - Start fresh from current Claude Code state:
    rm -rf ~/.claude-sync
    claude-sync init
```

**Exit code:** 1

---

### Edge Case 2: `~/.claude/` Doesn't Exist (Fresh Machine)

**Scenario:** User runs claude-sync before ever launching Claude Code.

**Behavior differs by command:**

| Command | Behavior |
|---------|----------|
| `init` | Error: "Run Claude Code at least once first" — `init` scans existing state, nothing to scan |
| `join` | Bootstrap minimal structure, then proceed — create empty `installed_plugins.json` and `known_marketplaces.json` |
| `pull` | Same as `join` — create minimal structure if needed |
| `status` | Informational: "Claude Code not initialized yet" |

**Bootstrap creates:**
```json
// ~/.claude/plugins/installed_plugins.json
{"version": 2, "plugins": {}}

// ~/.claude/plugins/known_marketplaces.json
{}
```

**Rationale:** `join` is the team onboarding path — new members shouldn't have to run Claude Code first. In practice, most users will have `~/.claude/` already.

---

### Edge Case 3: Plugin in Config No Longer Exists in Marketplace

**Scenario:** Marketplace maintainer removes a plugin, or marketplace URL changes.

**Behavior:** Warn and skip, continue with other plugins, track as "unavailable":

```
$ claude-sync pull

Pulled latest config.
  ✓ 22 plugins installed
  ✗ 1 plugin unavailable:
    • old-plugin@defunct-marketplace - marketplace not found
```

```
$ claude-sync status

UNAVAILABLE (marketplace no longer exists)
  ✗ old-plugin@defunct-marketplace
    To resolve:
      • Remove from config: edit config.yaml
      • Retry: claude-sync pull
```

**Key points:**
- Plugin stays in `config.yaml` (preserves user intent)
- Tracked locally as unavailable to avoid retry noise on every pull
- If plugin was already installed locally, it keeps working (cached in `~/.claude/plugins/cache/`)

**Exit code:** 0 (with warning) if other plugins succeeded; 2 if all failed

---

### Edge Case 4: Git Remote Unreachable

**Scenario:** Network outage, VPN required, corporate firewall.

**Behavior:** Fail with clear error; offer `--offline` flag:

```
$ claude-sync pull
Error: Cannot reach git remote.
  Repository: git@github.com:user/claude-sync-config.git
  Error: Could not resolve host: github.com

Options:
  - Check your network connection and try again
  - Run 'claude-sync pull --offline' to use local config.yaml
  - Run 'claude-sync status' to see current state

Exit code: 1
```

```
$ claude-sync pull --offline
Note: Skipping git pull (offline mode)

Reconciling with local config.yaml...
  ✓ 2 plugins installed
  ✓ Settings updated

Warning: Local config.yaml may differ from remote.
Run 'claude-sync pull' when online to sync.
```

**Rationale:** When user explicitly runs `pull`, they want remote state. SessionStart hook already degrades gracefully (outputs `{}` on failure).

---

### Edge Case 5: Concurrent Sessions Trigger Hook Simultaneously

**Scenario:** User opens 3 Claude Code windows at once; all trigger SessionStart hook.

**Behavior:** Timestamp-based skip with 30-second window:

```bash
# In session-start.sh
LAST_FETCH_FILE=~/.claude-sync/.last_fetch
FETCH_INTERVAL=30  # seconds

now=$(date +%s)
last_fetch=$(cat "$LAST_FETCH_FILE" 2>/dev/null || echo 0)

if [ $((now - last_fetch)) -gt $FETCH_INTERVAL ]; then
  git fetch --quiet 2>/dev/null || true
  echo "$now" > "$LAST_FETCH_FILE"
fi
```

**Key points:**
- First session fetches and writes timestamp
- Sessions opening within 30 seconds skip fetch
- Zero penalty for common single-window case
- If race occurs (multiple read before any write), worst case is all fetch — still safe

---

### Edge Case 6: Plugin Installation Fails Mid-Batch

**Scenario:** `pull` is installing 5 plugins; plugin 3 fails (network error, invalid plugin).

**Behavior:** Continue with remaining plugins, retry failed ones once, report all failures at end:

```
Installing plugins...
  ✓ context7@claude-plugins-official
  ✓ playwright@claude-plugins-official
  ✗ episodic-memory@superpowers-marketplace (network timeout)
  ✓ beads@beads-marketplace

Retrying failed plugins...
  ✓ episodic-memory@superpowers-marketplace

All plugins installed successfully.
```

Or if retry fails:

```
Retrying failed plugins...
  ✗ episodic-memory@superpowers-marketplace (marketplace unavailable)

⚠️ 3/4 plugins installed. 1 failed:
  • episodic-memory@superpowers-marketplace: marketplace unavailable

Run 'claude-sync pull' to retry failed plugins.
```

**Key points:**
- Plugins are independent — no need to stop on first failure
- Single retry handles transient network issues
- Exit code 0 if all succeed; exit code 2 if any fail after retry

---

## Distribution

### Initial Release

- **GitHub releases** - Binary downloads for macOS/Linux
- **Homebrew tap** - `brew install ruminaider/tap/claude-sync`
- **Claude Code plugin marketplace** - `claude-sync@claude-sync-marketplace`

### Future

- Potential adoption into `claude-plugins-official` if widely used
- Windows support via WSL or native port

---

## Resolved Questions

| Question | Resolution |
|----------|------------|
| **Marketplace API** | Git-based, no REST API. Query marketplace repos directly for version info. |
| **Settings merge behavior** | Implicit merge (scalars override, arrays union) + `$remove`/`$replace` escape hatches. |
| **Plugin installation** | Use `claude plugin install` CLI command. |
| **Update timing** | Notify + restart pattern. Updates require session restart (Claude Code limitation). |
| **Cross-platform** | macOS/Linux first. Windows/WSL deferred. |

---

## Appendix: Full CLI Reference

```
claude-sync - Sync Claude Code configuration across machines

USAGE:
    claude-sync [command]

CORE COMMANDS:
    init                Create new config from current setup
    join <url>          Join existing config repo
    status              Show sync status
    pull                Pull latest config
    push [-m "msg"]     Push changes to remote

PLUGIN COMMANDS (Phase 2):
    update              Apply available plugin updates
    fork <plugin>       Fork plugin for customization
    unfork <plugin>     Return to upstream
    pin <plugin> [ver]  Pin to specific version
    unpin <plugin>      Enable auto-updates

PROFILE COMMANDS (Phase 3):
    profile list        List profiles
    profile set <name>  Set profile for project
    profile show        Show active profile

VERSION COMMANDS (Phase 4):
    releases            List all versions
    release <version>   Create new release
    rollback [version]  Rollback to previous state
    diff <v1> <v2>      Compare versions
    log                 Show history

ADVANCED COMMANDS (Phase 4):
    upstream-diff       Show upstream changes for fork
    upstream-sync       Selectively apply upstream changes
    daemon start|stop   Background sync daemon
    preferences         Interactive preferences UI

FLAGS:
    -q, --quiet         Suppress output
    -v, --verbose       Verbose output
    --dry-run           Preview without applying
    --json              Output in JSON format
```
