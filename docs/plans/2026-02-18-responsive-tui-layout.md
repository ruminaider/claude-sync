# Responsive TUI Layout

## Problem

On smaller terminals (< 100 cols wide or < 28 rows tall), the init TUI breaks:
- Status bar shortcuts overflow to 2 lines, but layout math assumes 1 line
- This pushes the first sidebar entry ("Plugins") off the top of the screen
- Helper text consumes 3 lines of vertical space that's precious on small screens

## Approach: Responsive Breakpoints

Adapt individual components based on terminal dimensions. No architectural changes.

## Status Bar Width Tiers

| Width | Format | Shortcuts |
|-------|--------|-----------|
| >= 100 | Full | `Ctrl+S: save · Tab: profiles · /: search · ?: help · Esc: quit` |
| >= 80 | Short | `^S save · Tab profiles · / search · ? help · Esc quit` |
| < 80 | Minimal | `^S save · ? help · Esc quit` |

The status bar always fits in 1 line at any width.

## Helper Text Height Tiers

| Height | Helper | Lines |
|--------|--------|-------|
| >= 28 | Full (description + shortcuts + separator) | 3 |
| >= 24 | Compact (description + separator) | 2 |
| < 24 | Hidden | 0 |

`helperLines()` method replaces the `const helperHeight = 3`.

## Safety Guard

`View()` measures the actual rendered status bar height instead of assuming 1.
This prevents any future overflow from breaking the layout.

## Files

- `statusbar.go` — width-based shortcut tiers
- `root.go` — `helperLines()`, measured status bar height, compact helper rendering
- `root_test.go` — breakpoint tests
