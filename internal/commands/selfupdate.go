package commands

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"time"
)

const githubDownloadBase = "https://github.com/ruminaider/claude-sync/releases/download"

func buildAssetURL(version, goos, goarch string) string {
	return fmt.Sprintf("%s/v%s/claude-sync-%s-%s", githubDownloadBase, version, goos, goarch)
}

// SelfUpdate downloads and installs the latest version of claude-sync.
// currentVersion is the running binary's version (e.g., "0.11.0").
// If force is true, installs even if the current version matches.
func SelfUpdate(currentVersion string, force bool) error {
	latest, err := fetchLatestVersion()
	if err != nil {
		return fmt.Errorf("could not check for updates: %w", err)
	}

	if !force && !isNewer(latest, currentVersion) {
		fmt.Printf("claude-sync is up to date (v%s)\n", currentVersion)
		return nil
	}

	url := buildAssetURL(latest, runtime.GOOS, runtime.GOARCH)
	fmt.Printf("Downloading claude-sync v%s...\n", latest)

	execPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("could not determine binary path: %w", err)
	}
	execPath, err = filepath.EvalSymlinks(execPath)
	if err != nil {
		return fmt.Errorf("could not resolve binary path: %w", err)
	}

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return fmt.Errorf("download failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed: HTTP %d", resp.StatusCode)
	}

	// Write to temp file in the same directory (ensures same filesystem for rename).
	tmpFile, err := os.CreateTemp(filepath.Dir(execPath), "claude-sync-update-*")
	if err != nil {
		return fmt.Errorf("could not create temp file: %w\nYou may need to run with sudo.", err)
	}
	tmpPath := tmpFile.Name()

	// Clean up temp file on any failure.
	success := false
	defer func() {
		if !success {
			os.Remove(tmpPath)
		}
	}()

	if _, err := io.Copy(tmpFile, resp.Body); err != nil {
		tmpFile.Close()
		return fmt.Errorf("download interrupted: %w", err)
	}
	tmpFile.Close()

	// Preserve the original binary's permissions.
	info, err := os.Stat(execPath)
	if err != nil {
		return fmt.Errorf("could not stat current binary: %w", err)
	}
	if err := os.Chmod(tmpPath, info.Mode()); err != nil {
		return fmt.Errorf("could not set permissions: %w", err)
	}

	// Atomic replace.
	if err := os.Rename(tmpPath, execPath); err != nil {
		return fmt.Errorf("could not replace binary: %w\nYou may need to run with sudo.", err)
	}

	success = true
	fmt.Printf("Updated claude-sync: v%s -> v%s\n", currentVersion, latest)
	return nil
}
