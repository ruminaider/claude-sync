package main

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/ruminaider/claude-sync/internal/plugins"
)

// promptDuplicateResolution asks the user which source to keep for a duplicate plugin.
func promptDuplicateResolution(d plugins.Duplicate, syncDir string) (plugins.Resolution, error) {
	var choice string
	options := make([]huh.Option[string], len(d.Sources))
	for i, src := range d.Sources {
		options[i] = huh.NewOption(src, src)
	}

	err := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title(fmt.Sprintf("Duplicate plugin: %s — keep which source?", d.Name)).
				Options(options...).
				Value(&choice),
		),
	).Run()
	if err != nil {
		return plugins.Resolution{}, fmt.Errorf("aborted")
	}

	var removeSource string
	for _, src := range d.Sources {
		if src != choice {
			removeSource = src
			break
		}
	}

	var isActiveDev bool
	err = huh.NewForm(
		huh.NewGroup(
			huh.NewConfirm().
				Title("Are you actively developing this plugin locally?").
				Value(&isActiveDev),
		),
	).Run()
	if err != nil {
		isActiveDev = false
	}

	rel := "preference"
	localRepo := ""
	if isActiveDev {
		rel = "active-dev"
		err = huh.NewForm(
			huh.NewGroup(
				huh.NewInput().
					Title("Path to local repo:").
					Value(&localRepo),
			),
		).Run()
		if err != nil {
			return plugins.Resolution{}, fmt.Errorf("aborted")
		}
	}

	return plugins.Resolution{
		PluginName:   d.Name,
		KeepSource:   choice,
		RemoveSource: removeSource,
		Relationship: rel,
		LocalRepo:    localRepo,
	}, nil
}

// promptReEvaluation asks the user what to do about a stale active-dev plugin.
func promptReEvaluation(sig plugins.ReEvalSignal) (string, error) {
	title := fmt.Sprintf("%s: ", sig.PluginName)
	if sig.RepoMissing {
		title += "local repo no longer exists"
	} else if sig.LocalStale {
		title += "no local commits in 7+ days"
	}

	options := []huh.Option[string]{
		huh.NewOption("Switch to marketplace", "switch"),
		huh.NewOption("Keep local (reset timer)", "keep"),
	}
	// No snooze when repo is missing per design doc.
	if !sig.RepoMissing {
		options = append(options, huh.NewOption("Snooze 2 days", "snooze"))
	}

	var choice string
	err := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title(title).
				Options(options...).
				Value(&choice),
		),
	).Run()
	if err != nil {
		return "switch", nil
	}

	return choice, nil
}

// isForkDuplicate checks if a duplicate is a fork-vs-marketplace pair.
// Returns the fork source, marketplace source, and whether it matched.
func isForkDuplicate(d plugins.Duplicate) (forkSource, marketplaceSource string, ok bool) {
	if len(d.Sources) != 2 {
		return "", "", false
	}
	for _, src := range d.Sources {
		if strings.HasSuffix(src, "@"+plugins.MarketplaceName) {
			forkSource = src
		} else {
			marketplaceSource = src
		}
	}
	if forkSource != "" && marketplaceSource != "" {
		return forkSource, marketplaceSource, true
	}
	return "", "", false
}

// forkPreferenceResolution builds a Resolution that keeps the fork and disables the marketplace source.
func forkPreferenceResolution(name, forkSrc, mktSrc string) plugins.Resolution {
	return plugins.Resolution{
		PluginName:   name,
		KeepSource:   forkSrc,
		RemoveSource: mktSrc,
		Relationship: "preference",
	}
}

// promptDisableForkOriginal asks the user whether to disable the original marketplace source.
func promptDisableForkOriginal(forkSrc, mktSrc string) (bool, error) {
	var disable bool
	err := huh.NewForm(
		huh.NewGroup(
			huh.NewConfirm().
				Title(fmt.Sprintf("Disable original %s? (fork is active at %s)", mktSrc, forkSrc)).
				Affirmative("Yes").
				Negative("No").
				Value(&disable),
		),
	).Run()
	if err != nil {
		return false, fmt.Errorf("aborted")
	}
	return disable, nil
}
