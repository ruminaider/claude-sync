package tui

import "github.com/ruminaider/claude-sync/internal/commands"

// buildConfiguredMenu returns the flat menu list with section headers for a
// configured installation. The list is static -- dynamic status lives in the
// banner (Task 3), not in the menu items themselves.
func buildConfiguredMenu(_ commands.MenuState) []menuItem {
	return []menuItem{
		// ── Sync ──
		{label: "Sync", isHeader: true},
		{label: "Pull latest updates", actionID: ActionPull, mode: ModeCLI, hint: "\u23ce pull"},
		{label: "Push local changes", actionID: ActionPush, mode: ModeCLI, hint: "\u23ce push"},

		// ── Plugins ──
		{label: "Plugins", isHeader: true},
		{label: "Browse & install plugins", actionID: ActionBrowsePlugins, mode: ModeTUI},
		{label: "Remove a plugin", actionID: ActionRemovePlugin, mode: ModeTUI},
		{label: "Fork or customize a plugin", actionID: ActionForkPlugin, mode: ModeTUI},
		{label: "Pin a plugin version", actionID: ActionPinPlugin, mode: ModeTUI},

		// ── Config ──
		{label: "Config", isHeader: true},
		{label: "Switch profile", actionID: ActionProfileList, mode: ModeCLI},
		{label: "Edit full config", actionID: ActionConfigUpdate, mode: ModeTUI},
		{label: "Subscribe to another config", actionID: ActionSubscribe, mode: ModeTUI},
	}
}

// buildFreshInstallMenu returns the two-item menu for a fresh installation
// (no config directory found).
func buildFreshInstallMenu() []menuItem {
	return []menuItem{
		{label: "Create new config", actionID: "create-config", mode: ModeCLI, hint: "from this machine"},
		{label: "Join a shared config", actionID: "join-config", mode: ModeTUI, hint: "clone a repo"},
	}
}
