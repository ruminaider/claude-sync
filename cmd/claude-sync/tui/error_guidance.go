package tui

import "strings"

// ErrorHelp provides structured guidance for an error.
type ErrorHelp struct {
	Why    string // one sentence of context
	Action string // concrete next step
}

// ErrorGuidance returns contextual help for an error based on the action
// that triggered it and substrings in the error message. Returns nil when
// no specific guidance applies.
func ErrorGuidance(actionID string, err error) *ErrorHelp {
	if err == nil {
		return nil
	}
	msg := strings.ToLower(err.Error())

	switch actionID {
	case ActionPull:
		switch {
		case strings.Contains(msg, "not found in marketplace") ||
			strings.Contains(msg, "plugin not found") ||
			strings.Contains(msg, "404"):
			return &ErrorHelp{
				Why:    "Plugin may have been renamed or removed from the marketplace.",
				Action: "Edit config.yaml to update or remove the reference.",
			}
		case strings.Contains(msg, "could not resolve host") ||
			strings.Contains(msg, "network") ||
			strings.Contains(msg, "timeout") ||
			strings.Contains(msg, "connection refused") ||
			strings.Contains(msg, "authentication") ||
			strings.Contains(msg, "permission denied") ||
			strings.Contains(msg, "access denied") ||
			strings.Contains(msg, "repository not found"):
			return &ErrorHelp{
				Why:    "Remote repository could not be reached.",
				Action: "Check your network connection and GitHub authentication.",
			}
		case strings.Contains(msg, "conflict"):
			return &ErrorHelp{
				Why:    "Merge conflict detected during pull.",
				Action: "Run 'claude-sync conflicts' to resolve, then retry.",
			}
		}
	case ActionPush, ActionPushChanges:
		switch {
		case strings.Contains(msg, "non-fast-forward") ||
			strings.Contains(msg, "fetch first") ||
			strings.Contains(msg, "behind"):
			return &ErrorHelp{
				Why:    "Remote has changes you don't have locally.",
				Action: "Pull first, then push again.",
			}
		case strings.Contains(msg, "permission denied") ||
			strings.Contains(msg, "access denied") ||
			strings.Contains(msg, "forbidden"):
			return &ErrorHelp{
				Why:    "No write access to the remote repository.",
				Action: "Subscribers' changes stay local; ask a maintainer for access.",
			}
		case strings.Contains(msg, "no changes"):
			return &ErrorHelp{
				Why:    "Working tree matches the config repository.",
				Action: "Make changes locally, then push.",
			}
		}
	case ActionApprove:
		if strings.Contains(msg, "no pending") || strings.Contains(msg, "nothing to approve") {
			return &ErrorHelp{
				Why:    "No high-risk changes are waiting for review.",
				Action: "Pull to check for new upstream changes.",
			}
		}
	case ActionConflicts:
		if strings.Contains(msg, "no conflicts") {
			return &ErrorHelp{
				Why:    "All merge conflicts have already been resolved.",
				Action: "Pull or push to continue syncing.",
			}
		}
	case ActionConfigUpdate:
		if strings.Contains(msg, "no config") || strings.Contains(msg, "not found") {
			return &ErrorHelp{
				Why:    "No config.yaml found in the sync directory.",
				Action: "Run 'claude-sync init' or join a shared config first.",
			}
		}
	}

	return nil
}
