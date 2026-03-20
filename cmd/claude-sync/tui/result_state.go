package tui

// renderResultLines appends the standard success/error result display
// followed by "Press any key to go back". Returns the appended slice.
func renderResultLines(lines []string, success bool, msg string) []string {
	if success {
		lines = append(lines, stGreen.Render("\u2713 "+msg))
	} else {
		lines = append(lines, stRed.Render("\u2717 "+msg))
	}
	lines = append(lines, "")
	lines = append(lines, stDim.Render("Press any key to go back"))
	return lines
}

// resolveResultMsg returns the message to display from a result message.
// Prefers explicit message, falls back to error string, then "Unknown error".
func resolveResultMsg(success bool, message string, err error) string {
	if success {
		return message
	}
	if message != "" {
		return message
	}
	if err != nil {
		return err.Error()
	}
	return "Unknown error"
}
