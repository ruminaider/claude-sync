package subscriptions

// ResolveItems applies the category filter to determine which items are included.
// Resolution order:
//  1. Category mode: "all" starts with everything, "none" starts with nothing
//  2. Category-level include list (adds specific items to "none")
//  3. Top-level exclude map (removes specific items from "all")
//  4. Top-level include map (adds specific items to "none")
//
// Parameters:
//   - sub: the subscription with filter directives
//   - category: the category name ("mcp", "plugins", "settings", etc.)
//   - allItems: all available item names from the subscription source
//
// Returns the set of item names to include.
func ResolveItems(sub Subscription, category string, allItems []string) map[string]bool {
	mode := categoryMode(sub, category)

	result := make(map[string]bool)

	if mode == CategoryAll {
		// Start with everything.
		for _, item := range allItems {
			result[item] = true
		}
		// Apply excludes.
		if excludes, ok := sub.Exclude[category]; ok {
			for _, name := range excludes {
				delete(result, name)
			}
		}
	} else {
		// Start with nothing — only items explicitly included.
		if includes, ok := sub.Include[category]; ok {
			includeSet := make(map[string]bool, len(includes))
			for _, name := range includes {
				includeSet[name] = true
			}
			for _, item := range allItems {
				if includeSet[item] {
					result[item] = true
				}
			}
		}
	}

	return result
}

// categoryMode returns the effective CategoryMode for a given category.
func categoryMode(sub Subscription, category string) CategoryMode {
	switch category {
	case "mcp":
		return effectiveMode(sub.Categories.MCP)
	case "plugins":
		return effectiveMode(sub.Categories.Plugins)
	case "settings":
		return effectiveMode(sub.Categories.Settings)
	case "hooks":
		return effectiveMode(sub.Categories.Hooks)
	case "permissions":
		return effectiveMode(sub.Categories.Permissions)
	case "claude_md":
		return effectiveMode(sub.Categories.ClaudeMD)
	case "commands":
		if sub.Categories.Commands != nil {
			return effectiveMode(sub.Categories.Commands.Mode)
		}
		return CategoryNone
	case "skills":
		return effectiveMode(sub.Categories.Skills)
	default:
		return CategoryNone
	}
}

// effectiveMode returns the mode, defaulting to "none" if empty.
func effectiveMode(mode CategoryMode) CategoryMode {
	if mode == CategoryAll {
		return CategoryAll
	}
	return CategoryNone
}

// IsPreferred returns true if the subscription has a prefer directive for the
// given category and item name.
func IsPreferred(sub Subscription, category, itemName string) bool {
	items, ok := sub.Prefer[category]
	if !ok {
		return false
	}
	for _, name := range items {
		if name == itemName {
			return true
		}
	}
	return false
}
