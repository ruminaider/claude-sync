package commands

import (
	"os"
	"path/filepath"

	"github.com/ruminaider/claude-sync/internal/project"
)

// ProjectListEntry represents a single discovered project.
type ProjectListEntry struct {
	Path    string
	Profile string
}

// ProjectList scans the provided directories for projects with .claude/.claude-sync.yaml.
// It skips directories that don't have the config file and skips declined entries.
func ProjectList(searchDirs []string) ([]ProjectListEntry, error) {
	var entries []ProjectListEntry
	for _, dir := range searchDirs {
		cfg, err := project.ReadProjectConfig(dir)
		if err != nil {
			continue
		}
		if cfg.Declined {
			continue
		}
		entries = append(entries, ProjectListEntry{
			Path:    dir,
			Profile: cfg.Profile,
		})
	}
	return entries, nil
}

// ProjectListScan scans directories under the given parent dirs for projects.
// It looks one level deep for subdirectories containing .claude/.claude-sync.yaml.
func ProjectListScan(parentDirs []string) ([]ProjectListEntry, error) {
	var searchDirs []string
	for _, parent := range parentDirs {
		entries, err := os.ReadDir(parent)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.IsDir() {
				searchDirs = append(searchDirs, filepath.Join(parent, e.Name()))
			}
		}
	}
	return ProjectList(searchDirs)
}
