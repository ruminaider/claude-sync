# Picker Filter Bar

Add an always-visible text filter to the TUI picker, enabling users to narrow long lists by typing. Applies to all sections uniformly.

## Problem

Sections like Plugins (30), Permissions (173), and Cmds & Skills (growing) are painful to navigate by scrolling alone. No way to quickly find or bulk-select items matching a pattern.

## Design

### Layout

```
1. Profile tabs (Base / evvy / work / personal / +)
2. Filter bar ("Filter: ___              3/45")
3. Helper text ("Choose commands and skills to sync.")
4. Item list (scrollable, filtered)
5. Status bar ("2/2 Cmds & Skills selected")
```

The filter bar sits at the top of the right panel, above the helper text. Always visible in every section for consistency.

- **Left side:** `Filter:` label (dim) + typed text (section accent color) + cursor
- **Right side:** `N/M` count (visible items / total selectable), shown only when filter is non-empty
- **Empty state:** just `Filter:` with a dim cursor, minimal visual weight

### Keyboard Interaction

Printable characters go to the filter. Control keys navigate.

| Key | Action |
|-----|--------|
| Any letter/number/symbol | Appends to filter text |
| `Backspace` | Deletes last filter character |
| `Esc` | Clears filter (if non-empty). If empty, moves focus to sidebar |
| `Up` / `Down` | Navigate filtered list |
| `Space` | Toggle current item |
| `Enter` | Toggle current item (or trigger search action) |
| `Right` | Activate preview (only when filter is empty) |
| `Ctrl+A` | Select all visible items |
| `Ctrl+N` | Deselect all visible items |

**Sacrificed shortcuts:** `j`/`k` (navigation), `a`/`n` (select/deselect all), `h`/`l` (sidebar/preview aliases) become filter input characters. Arrow keys take over navigation. `Ctrl+A`/`Ctrl+N` replace `a`/`n`.

### Filtering Logic

- Case-insensitive substring match against `Display` + `Tag` combined
- Typing `beads` matches items tagged `[cmd] beads`; typing `skill` matches all `[skill]` items
- No fuzzy matching, just `strings.Contains`
- Headers shown only if they have at least one visible child; hidden otherwise
- `[+ Search projects]` virtual row always visible regardless of filter
- Cursor resets to first visible item when filter text changes
- `Ctrl+A`/`Ctrl+N` operate on filtered subset only

### Preview Interaction

When preview is active (right panel split), the filter bar remains visible but typing is disabled. Preview uses up/down for scrolling. Exiting preview returns to filter-active state.

### Helper Text

Updated to document new shortcuts:

```
ctrl+a select all  ctrl+n deselect all  type to filter  esc clear
```

Sections with preview also show `right preview`.

### Status Bar

No change. Shows total selection count (not filtered count).

## Files Changed

| File | Change |
|------|--------|
| `picker.go` | `filterText` field, `filteredItems()`, updated `Update()` key handling, `View()` with filter bar |
| `root.go` | Updated helper text strings for all sections |
| `root_test.go` | Updated tests for new helper text |

No new files. No changes to data model, search, init, pull, push, config, or profiles.

## Not In Scope

- Structured filter criteria (type/source toggle chips) -- future enhancement
- Fuzzy matching -- start with substring, iterate later
- Cross-section global search
