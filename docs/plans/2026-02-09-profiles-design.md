# Profile System Design

## Problem

`claude-sync init` creates a single flat config. Users with multiple contexts (work/personal) need different plugin sets, settings, and hooks per machine or project. Currently there's no way to express "these plugins are work-only" during setup.

## Data Model

Layered composition: a **base** config contains what every machine gets. **Profile** configs add to or remove from base. Each machine activates one profile.

### Repo Structure

```
~/.claude-sync/
├── config.yaml              # base config (always applied)
├── profiles/
│   ├── work.yaml            # adds/removes relative to base
│   └── personal.yaml        # adds/removes relative to base
├── active-profile           # single line: "work" (machine-local, gitignored)
├── user-preferences.yaml    # personal overrides (gitignored, already exists)
└── .gitignore
```

`active-profile` and `user-preferences.yaml` are gitignored (machine-local). Everything else is synced.

### Profile YAML Format

```yaml
# profiles/work.yaml
plugins:
  add:
    - greptile@claude-plugins-official
    - security-guidance@claude-plugins-official
  remove:
    - episodic-memory@superpowers-marketplace
    - ralph-wiggum@claude-plugins-official
settings:
  model: sonnet                # overrides base value
hooks:
  add:
    SessionStart: [{"matcher":"","hooks":[{"type":"command","command":"work-setup"}]}]
  remove:
    - PreCompact               # remove base hook by name
```

- `plugins.add` — plugin keys installed on top of base
- `plugins.remove` — base plugin keys excluded by this profile
- `settings` — key-value overrides (last wins over base)
- `hooks.add` — hooks added on top of base (key → raw JSON)
- `hooks.remove` — base hook names excluded by this profile

### Merge Resolution Order

Highest priority wins:

```
user-preferences.yaml   (local, never synced)
  ↑
profiles/<active>.yaml  (synced, machine-selected)
  ↑
config.yaml             (synced, base)
```

For plugins: start with base upstream/forked lists, apply profile adds (union), apply profile removes (subtract). For settings: deep merge, profile values override base. For hooks: start with base hooks, add profile hooks, remove listed hooks.

## Init Flow

Init is a two-phase process: base configuration, then optional profile creation.

### Phase 1: Base Configuration

```
Found 27 plugin(s)
Found settings: model
Found hooks: PreCompact (bd prime), SessionStart (bd prime)

First, let's configure your base — the default config every machine gets.

? Include all 27 plugins in base?
> Share all (Recommended)
  Choose which to share
  Don't share any plugins

[If "Choose which to share" → section picker with upstream/auto-forked groups]

? Include settings in base? (model)  →  Yes / No

? Sync base hooks?
> Share all
  Choose which to share
  Don't share hooks

Base configured: 20 plugins, model: opus, 2 hooks
```

This is the existing init flow with the new plugin picker. User finishes base, sees a summary, then decides about profiles.

### Phase 2: Profile Creation (Optional)

```
? Set up profiles? (e.g., work/personal layers on top of base)
> Yes
  No — just use this base config

[If No → write config.yaml, push, done — same as today]

[If Yes:]

? Create a profile:
> Work
  Personal
  Custom name...

[Profile "work":]

? Select plugins for "work" profile:
  ── Base (20) ─────────────────────
  [x] beads@beads-marketplace
  [x] context7@claude-plugins-official
  [ ] episodic-memory@superpowers        ← REMOVING from base
  ...
  ── Not in base (7) ───────────────
  [x] greptile@claude-plugins-official   ← ADDING
  [ ] ralph-wiggum@claude-plugins-official
  > [ Confirm ]

? Override model for "work"?  →  No / Yes (enter value)
? Override hooks for "work"?  →  Keep base hooks / Customize

? Create another profile?
> Personal
  Custom name...
  No more profiles

[Repeat for "personal"...]

? Create another profile?
> No more profiles

? Activate a profile on THIS machine?
> work
  personal
  None (base only)
```

Key behaviors:

- **"Set up profiles?" = No** → single-config flow, full backward compatibility
- Profile plugin picker shows ALL plugins — base ones pre-selected. Deselect a base plugin to remove it, select a non-base plugin to add it. The diff from base is computed automatically.
- First "Create a profile?" has no "No more profiles" option — user opted in, so at least one is required.
- Subsequent prompts include "No more profiles" as an option.
- After all profiles created, ask which to activate on this machine.
- `active-profile` is gitignored (machine-local choice).

### Profile Picker Diff Logic

The profile picker shows the full plugin list, with base plugins pre-selected. After confirmation:

```
selected_in_profile = picker result
base_plugins = base config plugins

adds = selected_in_profile - base_plugins     # in profile but not base
removes = base_plugins - selected_in_profile  # in base but not profile
```

This lets the user think in terms of "what should this profile's final set look like?" while we compute the adds/removes for the YAML.

## Join Flow

When joining a config that has profiles:

```
$ claude-sync join https://github.com/ruminaider/claude-sync-config.git

Cloning config...
✓ Cloned config repo

Applying base config...
✓ Installed 20 plugins
✓ Applied settings (model: opus)
✓ Applied hooks: PreCompact, SessionStart

Found 2 profiles available: work, personal
  Work:     +2 plugins, -1 plugin, model → sonnet
  Personal: +3 plugins, -2 plugins

? Activate a profile on this machine?
> work
  personal
  No — keep base only

Applying profile "work"...
✓ Added 2 plugins (greptile, security-guidance)
✓ Removed 1 plugin (episodic-memory)
✓ Updated settings (model: sonnet)
```

Key behaviors:

- Base is applied first — the user sees a concrete starting point.
- Profile activation is framed as an incremental layer on top of base.
- If no profiles exist in the config, join works exactly as today — no profile prompt.
- Profile choice stored in `active-profile` (gitignored).
- `claude-sync pull` uses the active profile when resolving what to apply.

## Pull Flow

`claude-sync pull` reads `active-profile` and applies the layered merge:

1. Pull latest from git
2. Parse `config.yaml` (base)
3. If `active-profile` exists, parse `profiles/<name>.yaml`
4. Merge: base + profile adds - profile removes
5. Apply resulting config to Claude Code

If active profile file references a profile that no longer exists (deleted upstream), warn and fall back to base only.

## Profile Management Commands

```bash
claude-sync profile list              # list available profiles
claude-sync profile show              # show active profile + resolved config
claude-sync profile set <name>        # activate profile on this machine
claude-sync profile set --none        # deactivate, use base only
```

`profile set` writes to `active-profile` file and re-applies config.

## Project-Level Overrides (Future)

Designed but not implemented in this phase. A project directory can override the machine-wide profile.

### Option A: Mapping File (Recommended)

`~/.claude-sync/project-mappings.yaml`:
```yaml
mappings:
  /Users/albert/work/*: work
  /Users/albert/hobby/*: personal
```

Supports glob patterns, centralized, optionally synced.

### Resolution Order

```
project-level profile  →  if match exists, use this
machine-level profile  →  fallback (active-profile file)
base only              →  fallback if no profile set
```

## Backward Compatibility

- `init` without profiles produces the same `config.yaml` as today — no `profiles/` directory.
- `join` on a config without profiles works as today — no profile prompt.
- `pull` without `active-profile` file applies base only.
- Existing `user-preferences.yaml` sits above profiles in the merge order and continues to work.
- The `IncludePlugins` field in `InitOptions` (nil = all) is unaffected.

## Files to Modify

### New files
- `internal/profiles/profile.go` — Profile struct, parse/marshal, merge logic
- `internal/profiles/profile_test.go` — Unit tests for merge
- `cmd/claude-sync/cmd_profile.go` — `profile list/show/set` subcommands

### Modified files
- `cmd/claude-sync/cmd_init.go` — Phase 2 profile creation loop after base config
- `cmd/claude-sync/cmd_join.go` — Profile detection + activation prompt after base apply
- `internal/commands/init.go` — Write profile YAMLs, update gitignore
- `internal/commands/join.go` — Read profiles, apply merge
- `internal/sync/sync.go` — Profile-aware merge in pull/apply
- `internal/config/config.go` — Add profile-related types if needed
- `.gitignore` template — Add `active-profile`

## Implementation Phases

### Phase 1 (Now): Init + Join with Profiles
- Profile data model and merge logic (`internal/profiles/`)
- Init flow: base config → optional profile creation loop
- Join flow: base apply → profile detection → activation prompt
- `profile list/show/set` commands
- Pull applies active profile merge

### Phase 2 (Future): Project-Level Overrides
- `project-mappings.yaml` with glob support
- Resolution: project mapping → machine profile → base
- `claude-sync profile set` with `--project` flag
