package main

import "github.com/charmbracelet/huh"

// confirmPrompt displays a yes/no confirmation dialog and returns the user's choice.
func confirmPrompt(title string) (bool, error) {
	var confirm bool
	err := huh.NewForm(
		huh.NewGroup(
			huh.NewConfirm().
				Title(title).
				Affirmative("Yes").
				Negative("No").
				Value(&confirm),
		),
	).Run()
	return confirm, err
}
