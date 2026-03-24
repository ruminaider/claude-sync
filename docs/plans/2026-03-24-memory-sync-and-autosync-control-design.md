# Memory.md Sync & Auto-Commit Control Design

## Overview

Two related features for claude-sync:

1. **Memory.md sync**: sync Claude Code memory files across machines at the user level, following the same patterns as CLAUDE.md (project-level sync deferred to v2)
2. **Auto-commit control**: give users control over whether new content auto-commits to the sync repo or requires explicit push

**Terminology:**
- **Memory.md** refers to the TUI display label; the actual file on disk is `MEMORY.md`
- **CCS** (Claude Code Switch) is an alternative launcher that supports multiple named instances
- **Auto-commit** is the mechanism (creating a local git commit in the sync repo); the user-facing concept is controlling what gets synced automatically

## Design Decisions

| Decision | Choice | Rationale |
|---|---|---|
| Source of truth | Sync repo (`~/.claude-sync/memory/`) | Same pattern as CLAUDE.md; sync repo owns canonical state |
| Scope for v1 | User-level memories only | Avoids cross-machine project identity mapping; project-level deferred to v2 |
| Architecture | Parallel systems (memory alongside claude-md) | File formats differ (frontmatter vs raw markdown); clean separation of concerns |
| Storage | Flat fragments with grouping metadata | Simple include/exclude; TUI renders tree from metadata |
| Fragment format | Memory files stored with YAML frontmatter intact | Preserves the format Claude Code expects |
| Auto-commit control | Per-category `all`/`tracked`/`manual` | Covers the full spectrum from fully automatic to fully manual |
| CLAUDE.md sub-sections | Split on `##` and `###` headers | Finer-grained control; child fragments grouped under parents |
| Pull targets | `~/.claude/memory/` + `~/.ccs/instances/*/memory/` | Works with both native Claude Code and CCS |
| TUI label | "Memory.md" (display), `memory` (config key) | Represents the Memory.md file system, not memory generally |

---

## Core Invariants

These invariants must be documented prominently in code and enforced in all code paths. They exist to protect user experimentation and local work.

1. **Pull never deletes local-only memory files.** Only synced fragments are written; files not in the sync repo are untouched.
2. **Pull never overwrites locally modified synced files.** Per-file hash protection (via `applied_hashes.go`) detects local edits and skips the file unless `--force` is passed.
3. **MEMORY.md regeneration includes all files in the directory.** The index merges synced fragments with any local-only `.md` files found in the same directory.
4. **New fragments are not auto-committed in `tracked` mode.** See Section 3 for the full auto-commit behavior matrix.

---

## 1. Storage Model

### Memory storage in sync repo

```
~/.claude-sync/
  memory/
    manifest.yaml
    user-prefers-terse.md
    feedback-no-mocks.md
    feedback-wheel-intake.md
```

### Manifest format

```yaml
# memory/manifest.yaml
fragments:
  user-prefers-terse:
    name: "Terse response preference"
    description: "User prefers concise responses without trailing summaries"
    type: user              # from memory frontmatter: user|feedback|project|reference
    level: user             # "user" or "project" (v1: user only)
    content_hash: "a1b2c3d4e5f6g7h8"
  feedback-no-mocks:
    name: "No database mocks"
    description: "Integration tests must hit real database"
    type: feedback
    level: user
    content_hash: "b2c3d4e5f6g7h8i9"
order:
  - user-prefers-terse
  - feedback-no-mocks
  - feedback-wheel-intake
```

Individual memory files are stored as-is with their YAML frontmatter intact (unlike CLAUDE.md fragments, which are raw markdown).

### Config reference

```yaml
# config.yaml
memory:
  include:
    - user-prefers-terse
    - feedback-no-mocks
    - feedback-wheel-intake
```

### Profile overlay

```yaml
# profiles/work.yaml
memory:
  add:
    - feedback-wheel-intake
  remove: []
```

---

## 2. CLAUDE.md Sub-Section Splitting

### Current behavior

The splitter in `claudemd.go:Split()` only parses `## ` headers. All content under a `## ` header (including `### ` sub-sections) becomes a single fragment.

### New behavior

The splitter parses both `## ` and `### ` headers. A section like:

```markdown
## Work Style

- Enter plan mode for non-trivial tasks

### Git Commits

Do NOT include Co-Authored-By lines

### Verification Supplements

When verifying completed work, also diff behavior...
```

Becomes three fragments:

| Fragment name | Header | Group |
|---|---|---|
| `work-style` | "Work Style" | `""` (top-level) |
| `work-style--git-commits` | "Git Commits" | `"work-style"` |
| `work-style--verification-supplements` | "Verification Supplements" | `"work-style"` |

**Naming convention:** child fragments use `{parent}--{child}` (double-hyphen separator) to make the relationship visible in filenames while keeping storage flat.

**Fragment content:** The parent fragment contains only the content between `## Work Style` and the first `### ` header. Child fragments each contain their `### ` header and content. Including/excluding a child does not duplicate content.

**Include list behavior:**
- Including `work-style` alone gets just the parent content (the intro paragraph)
- Including `work-style--git-commits` gets just that sub-section
- To get the full original section, include all three

**Assembly order:** Fragments are assembled in manifest `order` (parents before children), preserving the natural document flow.

**Manifest addition:** `FragmentMeta` gets a `group` field:

```yaml
# claude-md/manifest.yaml
fragments:
  work-style:
    header: "Work Style"
    content_hash: "..."
    group: ""
  work-style--git-commits:
    header: "Git Commits"
    content_hash: "..."
    group: "work-style"
  work-style--verification-supplements:
    header: "Verification Supplements"
    content_hash: "..."
    group: "work-style"
```

---

## 3. Auto-Commit Control

### Settings

```yaml
# user-preferences.yaml (machine-local, gitignored)
sync:
  auto_commit:
    claude_md: tracked     # all | tracked | manual
    memory: tracked        # all | tracked | manual
```

### Behavior by mode

| Mode | New fragments | Edits to synced fragments | Deletions |
|---|---|---|---|
| `all` | Auto-committed | Auto-committed | Auto-committed |
| `tracked` | Ignored (require explicit push) | Auto-committed | Ignored (require explicit push) |
| `manual` | Ignored | Ignored | Ignored |

### Default

New installations default to `tracked` for both categories. This preserves existing synced content without capturing experiments.

### Implementation

In `autocommit.go`, the existing code that unconditionally writes new CLAUDE.md fragments (lines 64-80) gets wrapped in a mode check:

```go
if len(reconcileResult.New) > 0 && autoCommitMode == "all" {
    // write new fragments + update config (current behavior)
}
// "tracked" mode: skip the New block entirely
// "manual" mode: skip the entire reconcile block
```

The same pattern applies to the new memory reconcile logic.

---

## 4. Import & Export Paths

### Import (local to sync repo)

**When:** `config create`, `config update`, or `claude-sync memory import`

**Flow:**
1. Scan `~/.claude/memory/` for all `.md` files (excluding MEMORY.md)
2. Optionally scan `~/.ccs/instances/*/memory/` if CCS is detected
3. For each file, read YAML frontmatter to get `name`, `description`, `type`
4. Generate fragment name from the `name` field (slugified)
5. Handle collisions: if two sources have a memory with the same slugified name, append a disambiguator
6. Copy the file into `~/.claude-sync/memory/` and update the manifest
7. Present TUI selection: which memories to include in base config vs assign to a profile

### Export / Apply (sync repo to local)

**When:** `pull`, `project init`

**Flow:**
1. Read `config.yaml` `memory.include`, merge with active profile (add/remove)
2. Read each included fragment from `~/.claude-sync/memory/{name}.md`
3. Write to `~/.claude/memory/` (with per-file hash protection)
4. If CCS detected, also write to `~/.ccs/instances/*/memory/` for each instance
5. Scan each target directory for ALL `.md` files (synced + local-only)
6. Regenerate MEMORY.md index from the full set

### MEMORY.md regeneration

MEMORY.md is rebuilt from all `.md` files present in the directory (both synced and local-only). The index is generated from each file's frontmatter fields (`name`, `description`, `type`). This means:
- Local-only memories always appear in the index
- Pull never causes experimental memories to disappear from the index
- The individual files are the source of truth; MEMORY.md is a derived artifact

---

## 5. Reconcile (Push Direction) for Memories

### Drift detection

Unlike CLAUDE.md (a single assembled file that must be split and diffed), memory files are already individual files on disk. This simplifies reconciliation.

**Push scan flow:**
1. Read all `.md` files (excluding MEMORY.md) from `~/.claude/memory/`; if CCS detected, also scan `~/.ccs/instances/*/memory/`
2. For each file, read YAML frontmatter, slugify the `name` field to a fragment name
3. Compare against `~/.claude-sync/memory/manifest.yaml`:
   - **Updated**: fragment exists in manifest, content hash differs
   - **New**: file on disk has no matching fragment in manifest
   - **Deleted**: fragment in manifest has no matching file on disk

4. Present results respecting the auto-commit mode configured in `user-preferences.yaml` (see Section 3 for the behavior matrix)

### Difference from CLAUDE.md reconcile

CLAUDE.md reconcile has rename detection (Jaccard similarity > 0.8) because users rename `## ` headers. For memories, the frontmatter `name` field is the stable identity, so rename detection is unnecessary. If the filename changes but the `name` field matches, matching happens on `name`.

### Collision handling

If two different source directories contain a memory with the same slugified name but different content, push flags it as a conflict and lets the user choose which version to keep.

---

## 6. Pull Flow Integration

### Target paths

| Memory level | Pull writes to |
|---|---|
| User-level (native) | `~/.claude/memory/` |
| User-level (CCS) | `~/.ccs/instances/*/memory/` (all detected instances) |

### CCS detection

On pull, claude-sync checks if `~/.ccs/instances/` exists. If so, it writes user-level memories to each detected instance's memory directory. This keeps native Claude Code and CCS in sync without user configuration.

### Pull steps

1. Read `config.yaml` `memory.include`, merge with active profile
2. For each included fragment, read from `~/.claude-sync/memory/{name}.md`
3. Write to `~/.claude/memory/` (skip if locally modified per hash check)
4. If CCS detected, write to each `~/.ccs/instances/*/memory/` (same hash protection)
5. Scan each target directory for all `.md` files (synced + local-only)
6. Regenerate MEMORY.md in each target directory from full set

### Pull result reporting

```go
type PullResult struct {
    // ... existing fields ...
    MemoryWritten  int
    MemorySkipped  int
    MemoryTotal    int
}
```

Displayed as: `Memory.md: 5 written, 1 skipped (locally modified)`

---

## 7. CLI Commands

### New commands

| Command | Description |
|---|---|
| `claude-sync memory import` | Scan local memory directories, present TUI selection, import to sync repo |
| `claude-sync memory list` | Show all memory fragments with name, type, and inclusion status |
| `claude-sync memory add <file>` | Import a specific memory file and add to `memory.include` |
| `claude-sync memory remove <name>` | Remove from `memory.include`; `--purge` also deletes fragment file |

### Modified commands

| Command | Change |
|---|---|
| `config create` / `config update` | Add memory import step after CLAUDE.md import |
| `push` | `PushScan` detects memory drift; respects auto-commit mode |
| `pull` | Add memory apply step after CLAUDE.md |
| `status` | Show memory sync status (fragments tracked, local drift) |

---

## 8. TUI Integration

### Config create/update: sidebar

Memory.md becomes the 9th sidebar section, positioned after CLAUDE.md:

```
  Plugins        5/10
  Settings       3/3
  CLAUDE.md      8/12
  Memory.md      4/6       <-- new
  Permissions    2/2
  MCP            3/5
  Cmds & Skills  4/7
  Keybindings    1/1
  Hooks          2/2
```

### Config create/update: Memory.md picker

Uses the existing Picker component with section headers grouping memories by `type`:

```
  | user (2)                                |
  |   [x] Terse response preference         |
  |   [x] Senior engineer background        |
  |                                          |
  | feedback (2)                             |
  |   [x] No database mocks in tests        |
  |   [ ] Wheel intake investigation         |
  |                                          |
  | reference (1)                            |
  |   [ ] Linear INGEST project              |
```

- Collapsible headers per memory type
- `user` and `feedback` types pre-selected by default on fresh import
- `project` and `reference` types unchecked by default
- Right-arrow shows full memory content in preview viewport

### Config create/update: CLAUDE.md sub-section tree

The Preview component for CLAUDE.md is enhanced with sub-section nesting:

```
  | [x] _preamble                           |
  | v [x] Work Style                        |
  |     [x] Git Commits                     |
  |     [x] Verification Supplements        |
  |     [ ] Self-Improvement Loop            |
  | v [x] Bash Commands                     |
  |     [x] Long-Running Commands            |
  | [x] Tool Authentication Requirements    |
```

- Parent `## ` sections are collapsible
- Toggling a parent auto-toggles all children
- Children are indented with visual nesting indicators
- Individual children can be toggled independently
- Right-side preview shows content of the focused fragment

### Profile tab behavior

When switching to a profile tab:
- Base selections shown with inherited marker (read-only)
- Profile can add additional memories or remove base ones
- Same add/remove pattern as every other section

### Push TUI: memory drift

```
  | Memory.md Changes                        |
  |                                          |
  | Updated:                                 |
  |   [x] user-prefers-terse (user)          |
  |   [x] feedback-no-mocks (feedback)       |
  |                                          |
  | New (not yet synced):                    |
  |   [ ] feedback-wheel-intake (feedback)   |
  |   [ ] reference-linear-ingest (reference)|
  |                                          |
  | Removed locally:                         |
  |   [ ] project-old-notes (project)        |
```

- Updated memories checked by default (already tracked)
- New memories unchecked by default (user opts in)
- Removed memories unchecked (checking confirms deletion from sync repo)
- Each entry shows memory `type` in parentheses
- Respects auto-commit mode: in `tracked` mode, only "Updated" appears

### Save overlay

```
  | Plugins:       5 upstream, 2 forked      |
  | Settings:      3 keys                    |
  | CLAUDE.md:     8 fragments (3 sub-sections)|
  | Memory.md:     4 fragments               |
  | Permissions:   2 allow rules             |
  | MCP:           3 servers                 |
  | Cmds & Skills: 4 commands, 3 skills      |
  | Keybindings:   1 override                |
  | Hooks:         2 hooks                   |
```

---

## 9. Components to Build or Modify

### New packages

- **`internal/memory/`**: import, reconcile, manifest read/write (parallel to `internal/claudemd/`)

### Modified packages

| Package | Changes |
|---|---|
| `internal/config/` | Add `MemoryConfig` struct, `memory` key parsing/marshaling, `CategoryMemory` constant |
| `internal/profiles/` | Add `ProfileMemory` struct, `MergeMemory()` function |
| `internal/project/` | Add `ProjectMemoryOverrides` (stub for v2) |
| `internal/commands/autocommit.go` | Add memory reconcile with mode check; wrap existing CLAUDE.md new-fragment logic in mode check |
| `internal/commands/pull.go` | Add memory apply step |
| `internal/commands/push.go` | Add memory drift detection to `PushScan`/`PushApply` |
| `internal/commands/init.go` | Add memory import to `InitScan`/`buildAndWriteConfig` |
| `internal/commands/applied_hashes.go` | Add per-memory-file hash keys |
| `internal/paths/paths.go` | Add memory directory paths, CCS detection helpers |
| `internal/claudemd/claudemd.go` | Enhance `Split()` to handle `### ` headers; add `group` to `FragmentMeta` |

### TUI changes

| File | Changes |
|---|---|
| `cmd/claude-sync/tui/root.go` | Add "Memory.md" sidebar section, wire to picker |
| `cmd/claude-sync/tui/picker.go` | Memory picker with type-based grouping |
| `cmd/claude-sync/tui/preview.go` | Enhance CLAUDE.md preview with sub-section tree (indentation, parent toggle) |
| `cmd/claude-sync/tui/overlay.go` | Add memory count to summary overlay |

### Config additions

```yaml
# config.yaml
memory:
  include: [...]

# user-preferences.yaml
sync:
  auto_commit:
    claude_md: tracked
    memory: tracked
```

---

## 10. Future Work (v2)

These items are explicitly out of scope for v1 but inform the design:

- **Project-level memory sync**: requires project identity mapping (git remote URL + user-assigned name) and pull-side path resolution
- **Instance-to-profile automatic mapping**: auto-match CCS instances to claude-sync profiles
- **Memory deduplication**: detect when the same memory exists across multiple projects and consolidate
- **Conflict resolution UI**: richer merge tooling when the same memory diverges across machines
