# Profile List Overlay Design

## Problem

After choosing "With profiles" in `claude-sync init`, the user names one profile at a time via a text input overlay. There's no way to define all desired profiles upfront before configuring them.

## Design

New overlay type `OverlayProfileList` — a vertical list of text input fields with a Done button.

### Visual

```
┌─ Profile names ───────────────────────┐
│                                       │
│  Base config is created automatically │
│  Add your profiles:                   │
│                                       │
│    work                               │
│  > personal_                          │
│                                       │
│  Enter: add  ⌫ empty: remove          │
│                                       │
│           [ Done ]                    │
│                                       │
└───────────────────────────────────────┘
```

### Key Bindings

| Key | Action |
|-----|--------|
| Up/Down | Move between name fields and Done button |
| Enter on name field | Add new blank row below, focus it |
| Enter on Done | Submit all valid names |
| Backspace on empty field | Remove that row (min 1 row) |
| Esc | Cancel — back to config style choice |

### State

- `inputs []textinput.Model` — one per profile name row
- `activeLine int` — which input has focus (or len(inputs) for Done button)
- `onDone bool` — true when cursor is on the Done button

### Validation on Submit

- Strip empty rows
- Trim whitespace, lowercase
- Reject "base" (reserved)
- Reject duplicates (keep first occurrence)
- At least 1 valid name required

### Return Value

`OverlayCloseMsg` gains a `Results []string` field (nil for other overlay types).

## Integration

1. User picks "With profiles" → show `ProfileListOverlay` (replaces single `TextInputOverlay`)
2. User adds names, hits Done → `handleOverlayClose` receives `Results: ["work", "personal"]`
3. Loop `createProfile(name)` for each name
4. Switch to Base tab (`m.activeTab = "Base"`)
5. Tab bar shows `[Base] [work] [personal] [+]`

The "+" tab in the main TUI keeps the existing single-name `TextInputOverlay`.

## Files Changed

| File | Change |
|------|--------|
| `overlay.go` | Add `OverlayProfileList` type, fields, update/view methods, constructor |
| `overlay.go` | Add `Results []string` to `OverlayCloseMsg` |
| `root.go` | New context `overlayProfileNames`, replace handler, loop `createProfile()` |
| `overlay_test.go` | Tests for new overlay type |
| `root_test.go` | Update profile creation flow test |
