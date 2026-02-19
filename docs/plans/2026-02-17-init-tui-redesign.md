# Init Wizard TUI Redesign

## Problem

The current init wizard is a linear step-by-step flow with 22 steps. This causes several UX issues:

1. **No CLAUDE.md preview** — can't see section contents before deciding to include/exclude
2. **Missing CLAUDE.md sources** — only scans `~/.claude/CLAUDE.md`, misses project-level `.claude/CLAUDE.md` files
3. **No base CLAUDE.md configuration** — base config phase skips CLAUDE.md section selection (only profiles get granular control)
4. **Linear flow is rigid** — forced to make decisions in a fixed order, can't revisit sections or compare across profiles
5. **No cross-profile comparison** — can't see what base has vs what a profile adds/removes without going through the whole flow

## Design

### Layout

```
┌─────────────────────────────────────────────────────────────┐
│  [Base]  [work]  [personal]  [+]                            │  ← Tab bar (profiles)
├──────────────┬──────────────────────────────────────────────┤
│              │                                              │
│  Plugins     │  ── Upstream (20) ──                         │
│  Settings    │  > [x] claude-code-setup@claude-plugins...   │
│  CLAUDE.md   │    [x] code-review@claude-plugins-official   │
│  Permissions │    [ ] beads@beads-marketplace                │
│  MCP         │    [x] context7@claude-plugins-official      │
│  Keybindings │    ...                                       │
│  Hooks       │  ── Auto-forked (2) ──                       │
│              │    [ ] figma-minimal@figma-minimal-market...  │
│              │    [ ] interactive-review@interactive-rev...  │
│              │                                              │
│              │                                    [Confirm] │
├──────────────┴──────────────────────────────────────────────┤
│  20/27 plugins selected · Base config                       │  ← Status bar
└─────────────────────────────────────────────────────────────┘
```

When the **CLAUDE.md** section is selected, the content pane splits into a preview layout:

```
├──────────────┬─────────────────┬────────────────────────────┤
│              │ CLAUDE.md       │ ## Subagent-Driven Dev...   │
│  Plugins     │                 │                             │
│  Settings    │ [x] (preamble)  │ Always use subagent-driven  │
│  CLAUDE.md   │ [x] Subagent... │ development when working:   │
│  Permissions │ [x] Tool Auth.. │                             │
│  MCP         │ [ ] Bash Cmds   │ 1. **Planning**: Use the    │
│  Keybindings │ [x] Color Sch.. │    Task tool with agents    │
│  Hooks       │ [x] Git Commits │ 2. **Execution**: Use the   │
│              │ [x] Generated.. │    Task tool with agents    │
│              │ [x] README Wri. │ 3. **Review**: Use review    │
│              │                 │    agents to validate       │
│              │ [+ Search...]   │                             │
│              │                 │                             │
├──────────────┴─────────────────┴────────────────────────────┤
│  7/8 sections selected · Base config                        │
└─────────────────────────────────────────────────────────────┘
```

### Components

#### Tab Bar (top)

- Renders profile tabs: `[Base]  [work]  [personal]  [+]`
- `[+]` opens a name input to create a new profile
- Switching tabs updates the content pane to show that profile's selections
- Profile tabs show diff-aware state: for non-base profiles, items are shown relative to base (added/removed markers)
- Keybinding: `Tab`/`Shift+Tab` to cycle tabs, or number keys `1`-`9`

#### Sidebar (left)

- Vertical list of config sections: Plugins, Settings, CLAUDE.md, Permissions, MCP, Keybindings, Hooks
- Each entry shows a summary count: `Plugins (20/27)`, `Settings (3)`, `CLAUDE.md (7/8)`
- Sections with no data from the scan are grayed out / hidden
- Keybinding: `Up`/`Down` or `j`/`k` when sidebar is focused

#### Content Pane (right)

- Renders the picker/editor for the currently selected section + profile combination
- For most sections: a multi-select picker with section headers (reusing existing picker patterns)
- For CLAUDE.md: split into list (left) + preview (right)
- For simple sections (Keybindings): a confirm toggle or key=value editor

#### Status Bar (bottom)

- Shows context: `{count} selected · {profile name} config`
- Shows keyboard shortcuts: `Tab: switch profile · /: search · a: all · n: none · Enter: toggle`

#### Focus Management

Four focus zones cycled with a global key (e.g., `Ctrl+H` for sidebar, `Ctrl+L` for content, `Ctrl+K` for tabs):

1. **Tab bar** — left/right to switch profiles
2. **Sidebar** — up/down to switch sections
3. **Content pane** — up/down to navigate items, space/enter to toggle
4. **Preview pane** — scroll only (CLAUDE.md section), not interactive

Default focus starts on the sidebar. Pressing `Enter` or `Right` on a sidebar item focuses the content pane. `Esc` from content returns focus to sidebar.

### CLAUDE.md Sources

The scan phase discovers CLAUDE.md files from multiple sources:

1. **User-level**: `~/.claude/CLAUDE.md` — always scanned
2. **Project discovery**: `[+ Search projects]` action in the CLAUDE.md section triggers a background search

#### Background Search

When the user activates `[+ Search projects]`:

1. Run `fd -t f -d 4 "CLAUDE.md" --search-path ~ -E node_modules -E .git -E Library -E .cache` (with `find` fallback if `fd` not available)
2. Filter results to only `.claude/CLAUDE.md` paths
3. Results populate incrementally as they're found (via `tea.Cmd` messages)
4. Each result shown as the project path (e.g., `~/Work/evvy`)
5. User selects which project CLAUDE.md files to import sections from
6. Selected files are split into sections and merged into the picker list, grouped by source

The search caps at depth 4 and excludes: `node_modules`, `.git`, `Library`, `.cache`, `.Trash`, `go/pkg`.

### Profile Behavior

#### Base Tab

Shows all items from the scan. Selections here define the shared baseline. All items start pre-selected (opt-out pattern, matching current behavior).

#### Profile Tabs

Show items relative to base:

- Items in base are shown with a `[base]` tag and start selected
- Items not in base are shown in a separate "Available" section, unselected
- Deselecting a base item adds it to the profile's `Remove` list
- Selecting a non-base item adds it to the profile's `Add` list

This matches the current profile diff model (`Add`/`Remove` lists) but makes it visual.

#### New Profile (`[+]`)

1. Opens a text input overlay for the profile name
2. Offers presets (Work, Personal) that aren't already used
3. New tab appears, initialized with base selections
4. User customizes per-section

### Section-Specific Behavior

#### Plugins

- Sections: "Upstream ({n})" / "Auto-forked ({n})"
- Profile view adds: "Base ({n})" / "Not in base ({n})" grouping
- Display: plugin key (e.g., `beads@beads-marketplace`)

#### Settings

- Flat list, displayed as `key: value` (values are small — model name, env string, etc.)
- Profile view: base settings shown, with ability to override values (not just include/exclude)
- Model override: inline text input next to the setting

#### CLAUDE.md

- Split pane: section list (left) + content preview (right)
- Sections from user-level CLAUDE.md grouped under "~/.claude/CLAUDE.md"
- Sections from project CLAUDE.md grouped under project path
- `[+ Search projects]` button at bottom of list triggers discovery
- Preview updates as cursor moves through sections
- Profile view: can add/remove sections relative to base

#### Permissions

- Sections: "Allow ({n})" / "Deny ({n})"
- Display: rule text (e.g., `Bash(git *)`)
- Profile view: base rules shown, plus ability to add extra allow/deny rules via text input

#### MCP

- Flat list of server names
- Display: server name (e.g., `newrelic`, `slack`)
- Profile view: "Base ({n})" / "Not in base ({n})" grouping

#### Keybindings

- Simple toggle: include yes/no
- If yes, all keybindings included (no per-binding selection — keybindings are typically small)
- Profile view: override toggle + key=value input for overrides

#### Hooks

- Flat list displayed as `HookName: command` (extracted from JSON for readability)
- Profile view: base hooks shown, with ability to remove specific hooks

### Lifecycle

#### Startup

1. Run `InitScan()` (existing business logic, unchanged)
2. If scan finds nothing → show message and exit
3. Render TUI with Base tab, sidebar focused, first non-empty section selected
4. If `--skip-profiles` flag: no tab bar, just Base

#### Profile Creation Prompt

On first render (if `--skip-profiles` not set), show an overlay:

```
┌─────────────────────────────┐
│  Configuration style:       │
│                             │
│  > Simple (single config)   │
│    With profiles            │
│                             │
└─────────────────────────────┘
```

If "With profiles" selected, prompt for initial profile names (same preset logic as today), then render tabs.

If "Simple" selected, no tab bar — just Base content.

#### Confirmation

- Global `Ctrl+S` or a `[Save & Init]` button in the status bar
- Before saving, validate: at least something selected across all sections
- Shows a summary overlay:

```
┌──────────────────────────────────────┐
│  Ready to initialize claude-sync?    │
│                                      │
│  Base: 20 plugins, 3 settings,       │
│        7 CLAUDE.md sections,         │
│        193 permissions, 2 hooks      │
│                                      │
│  Profiles: work, personal            │
│                                      │
│  [Cancel]              [Initialize]  │
└──────────────────────────────────────┘
```

#### Post-Init

After Init() succeeds, show result summary and exit TUI. Same output as current wizard (plugin counts, settings applied, etc.).

### Component Architecture (bubbletea)

```
rootModel
├── tabBar        (profile tabs + [+] button)
├── sidebar       (section list with counts)
├── contentPane   (delegates to section-specific models)
│   ├── pluginPicker
│   ├── settingsEditor
│   ├── claudeMDPicker   (contains splitPane with preview)
│   ├── permissionsPicker
│   ├── mcpPicker
│   ├── keybindingsEditor
│   └── hooksPicker
├── statusBar     (context + shortcuts)
└── overlay       (modals: profile name, confirmation, search)
```

**Message Flow:**
- `tea.WindowSizeMsg` → rootModel distributes dimensions to all children
- `tea.KeyMsg` → rootModel routes to focused component
- Focus change messages bubble up from children, rootModel updates focus state
- Tab switch messages → rootModel swaps content pane data context
- Section switch messages → rootModel swaps content pane model

**State:**
- `scanResult *InitScanResult` — read-only scan data
- `baseSelections map[Section]Selection` — what's selected for base
- `profileSelections map[string]map[Section]Selection` — per-profile overrides
- `focusZone FocusZone` — which component has focus
- `activeTab string` — current profile tab ("base" or profile name)
- `activeSection Section` — current sidebar section

### Files

| File | Purpose |
|------|---------|
| `cmd/claude-sync/tui/root.go` | Root model, focus management, layout composition |
| `cmd/claude-sync/tui/tabbar.go` | Tab bar component |
| `cmd/claude-sync/tui/sidebar.go` | Sidebar navigation component |
| `cmd/claude-sync/tui/picker.go` | Generic multi-select picker (replaces current picker.go) |
| `cmd/claude-sync/tui/preview.go` | CLAUDE.md split-pane preview |
| `cmd/claude-sync/tui/overlay.go` | Modal overlays (confirm, name input, search) |
| `cmd/claude-sync/tui/statusbar.go` | Status bar with context and shortcuts |
| `cmd/claude-sync/tui/search.go` | Background CLAUDE.md file search |
| `cmd/claude-sync/tui/styles.go` | Shared lipgloss styles |
| `cmd/claude-sync/cmd_init.go` | Refactored to launch TUI and collect results |

### Migration

The existing `cmd_init.go` (1500+ lines of step machine) gets replaced by the TUI. The business logic (`internal/commands/init.go` — `InitScan`, `Init`, `InitOptions`) stays unchanged. The TUI collects the same data and builds the same `InitOptions` struct.

The existing `picker.go` can be kept for other commands (push, mcp import, etc.) that use simple pickers. The new TUI picker in `tui/picker.go` is a more capable version used only within the init TUI.

### Decisions

1. **Terminal size**: 80x24 minimum. Dynamic resize via `tea.WindowSizeMsg` — layout recalculates on every resize. Below minimum, show a "please resize your terminal" message. Can tune thresholds later.

2. **Profile deletion**: Supported. `Ctrl+D` when tab bar is focused shows a confirm overlay "Delete profile '{name}'?". Base tab cannot be deleted.

3. **Profile activation removed from init**: Profile activation is a pull-side concern — the machine running init is defining config, not choosing which profile to use. Activation belongs in `claude-sync join`, `claude-sync profile activate`, or a pull-time prompt. Removed from the init TUI entirely.

4. **Existing config pre-loading**: Re-running init pre-loads existing `config.yaml` + profiles as the default state. User sees what's currently configured and can adjust. A `Ctrl+R` "Reset to defaults" action clears back to scan-based defaults (as if config didn't exist). This makes init the single entry point for both creating and editing sync config.
