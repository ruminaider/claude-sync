package tui

// Action ID constants for menu items.
const (
	ActionPull          = "pull"
	ActionPush          = "push"
	ActionBrowsePlugins = "browse-plugins"
	ActionRemovePlugin  = "remove-plugin"
	ActionForkPlugin    = "fork-plugin"
	ActionPinPlugin     = "pin-plugin"
	ActionProfileList   = "switch-profile"
	ActionConfigUpdate  = "edit-config"
	ActionSubscribe     = "subscribe"
)

// AllActionIDs returns every action ID that the menu can emit.
func AllActionIDs() []string {
	return []string{
		ActionPull,
		ActionPush,
		ActionBrowsePlugins,
		ActionRemovePlugin,
		ActionForkPlugin,
		ActionPinPlugin,
		ActionProfileList,
		ActionConfigUpdate,
		ActionSubscribe,
	}
}

// menuItemMode describes how an action should be executed.
type menuItemMode int

const (
	// ModeCLI means the action runs as a CLI command (inline with spinner).
	ModeCLI menuItemMode = iota
	// ModeTUI means the action opens a sub-view in the TUI.
	ModeTUI
)

// menuItem is a single entry in the flat menu list.
// Items with isHeader=true are non-selectable section dividers.
type menuItem struct {
	label    string       // display text
	actionID string       // action ID emitted on selection (empty for headers)
	mode     menuItemMode // CLI or TUI
	isHeader bool         // true = section divider, not selectable
	hint     string       // optional right-aligned hint text
}
