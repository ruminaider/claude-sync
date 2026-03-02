package tui

// ActionType distinguishes how a menu action is executed.
type ActionType int

const (
	ActionNone ActionType = iota // category (has children, no action)
	ActionCLI                    // exit menu, run CLI command, re-enter menu
	ActionTUI                    // exit menu, launch sub-TUI, re-enter menu
)

// Action ID constants — single source of truth for menu item <-> dispatcher mapping.
// Adding a new menu action requires adding a constant here first.
const (
	// Phase 1
	ActionPull          = "pull"
	ActionPush          = "push"
	ActionStatus        = "status"
	ActionConfigCreate  = "config-create"
	ActionConfigUpdate  = "config-update"
	ActionConfigJoin    = "config-join"
	ActionSetup         = "setup"
	ActionSubscribe     = "subscribe"
	ActionSubscriptions = "subscriptions"
	ActionProfileList   = "profile-list"
	ActionProfileShow   = "profile-show"
	ActionApprove       = "approve"
	ActionReject        = "reject"
	ActionMCPImport     = "mcp-import"
	ActionProjects      = "projects"
	ActionConflicts     = "conflicts"

	// Phase 2
	ActionPluginPin     = "plugin-pin"
	ActionPluginUnpin   = "plugin-unpin"
	ActionPluginFork    = "plugin-fork"
	ActionPluginUnfork  = "plugin-unfork"
	ActionPluginUpdate  = "plugin-update"
	ActionProfileSet    = "profile-set"
	ActionProjectInit   = "project-init"
	ActionProjectRemove = "project-remove"
)

// AllActionIDs returns all known action ID constants. Used by tests to verify
// dispatch coverage — every ID here must have a case in dispatchAction.
func AllActionIDs() []string {
	return []string{
		ActionPull, ActionPush, ActionStatus,
		ActionConfigCreate, ActionConfigUpdate, ActionConfigJoin, ActionSetup,
		ActionSubscribe, ActionSubscriptions,
		ActionProfileList, ActionProfileShow,
		ActionApprove, ActionReject,
		ActionMCPImport, ActionProjects, ActionConflicts,
		// Phase 2
		ActionPluginPin, ActionPluginUnpin, ActionPluginFork, ActionPluginUnfork,
		ActionPluginUpdate, ActionProfileSet, ActionProjectInit, ActionProjectRemove,
	}
}

// MenuAction is the result of selecting a leaf menu item.
type MenuAction struct {
	ID   string     // one of the Action* constants above
	Type ActionType
	Args []string   // optional arguments for parameterized actions
}

// menuItem represents one entry in the menu tree.
type menuItem struct {
	label    string
	desc     string     // short description shown to the right
	children []menuItem // non-nil = category, nil = leaf action
	action   MenuAction // only for leaf items
}

// isCategory returns true if this item has children (is a submenu).
func (m menuItem) isCategory() bool {
	return len(m.children) > 0
}
