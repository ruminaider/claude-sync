package tui

import "github.com/ruminaider/claude-sync/internal/commands"

// BuildMenuItems returns the menu item tree based on the detected state.
func BuildMenuItems(state commands.MenuState) []menuItem {
	if !state.ConfigExists {
		return buildFreshInstallMenu()
	}
	return buildConfiguredMenu(state)
}

func buildFreshInstallMenu() []menuItem {
	return []menuItem{
		{
			label:  "Create new config",
			desc:   "from this machine's Claude Code setup",
			action: MenuAction{ID: ActionConfigCreate, Type: ActionTUI},
		},
		{
			label:  "Join existing config",
			desc:   "clone a shared config repo",
			action: MenuAction{ID: ActionConfigJoin, Type: ActionTUI},
		},
	}
}

func buildConfiguredMenu(state commands.MenuState) []menuItem {
	return []menuItem{
		buildSyncCategory(),
		buildConfigCategory(),
		buildPluginsCategory(),
		buildProfilesCategory(state),
		buildAdvancedCategory(state),
	}
}

func buildSyncCategory() menuItem {
	return menuItem{
		label: "Sync",
		children: []menuItem{
			{label: "Pull latest config", action: MenuAction{ID: ActionPull, Type: ActionCLI}},
			{label: "Push local changes", action: MenuAction{ID: ActionPush, Type: ActionCLI}},
			{label: "View sync status", action: MenuAction{ID: ActionStatus, Type: ActionCLI}},
		},
	}
}

func buildConfigCategory() menuItem {
	return menuItem{
		label: "Config",
		children: []menuItem{
			{label: "Edit config", desc: "modify synced settings", action: MenuAction{ID: ActionConfigUpdate, Type: ActionTUI}},
			{label: "Setup auto-sync", desc: "register session hooks", action: MenuAction{ID: ActionSetup, Type: ActionCLI}},
		},
	}
}

func buildPluginsCategory() menuItem {
	return menuItem{
		label: "Plugins",
		children: []menuItem{
			{label: "Subscribe", desc: "follow another config", action: MenuAction{ID: ActionSubscribe, Type: ActionTUI}},
			{label: "List subscriptions", action: MenuAction{ID: ActionSubscriptions, Type: ActionCLI}},
		},
	}
}

func buildProfilesCategory(state commands.MenuState) menuItem {
	return menuItem{
		label: "Profiles",
		children: []menuItem{
			{label: "List profiles", action: MenuAction{ID: ActionProfileList, Type: ActionCLI}},
			{label: "Show active profile", action: MenuAction{ID: ActionProfileShow, Type: ActionCLI}},
		},
	}
}

func buildAdvancedCategory(state commands.MenuState) menuItem {
	var children []menuItem

	if state.HasPending {
		children = append(children,
			menuItem{label: "Approve pending changes", action: MenuAction{ID: ActionApprove, Type: ActionCLI}},
			menuItem{label: "Reject pending changes", action: MenuAction{ID: ActionReject, Type: ActionCLI}},
		)
	}

	children = append(children,
		menuItem{label: "Import MCP servers", action: MenuAction{ID: ActionMCPImport, Type: ActionCLI}},
		menuItem{label: "Manage projects", action: MenuAction{ID: ActionProjects, Type: ActionCLI}},
	)

	if state.HasConflicts {
		children = append(children,
			menuItem{label: "Resolve conflicts", action: MenuAction{ID: ActionConflicts, Type: ActionCLI}},
		)
	}

	return menuItem{
		label:    "Advanced",
		children: children,
	}
}
