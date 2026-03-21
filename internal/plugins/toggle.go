package plugins

import (
	"encoding/json"

	"github.com/ruminaider/claude-sync/internal/claudecode"
)

// ToggleEnabledPlugin sets the enabled state for a single plugin key in settings.json.
func ToggleEnabledPlugin(claudeDir, pluginKey string, enabled bool) error {
	settings, err := claudecode.ReadSettings(claudeDir)
	if err != nil {
		return err
	}

	var ep map[string]bool
	raw, ok := settings["enabledPlugins"]
	if !ok {
		ep = make(map[string]bool)
	} else {
		if err := json.Unmarshal(raw, &ep); err != nil {
			return err
		}
	}

	ep[pluginKey] = enabled

	updated, err := json.Marshal(ep)
	if err != nil {
		return err
	}
	settings["enabledPlugins"] = updated
	return claudecode.WriteSettings(claudeDir, settings)
}
