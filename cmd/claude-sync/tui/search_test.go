package tui

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestShortenPath(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("cannot determine home dir")
	}

	tests := []struct {
		input    string
		expected string
	}{
		{filepath.Join(home, "Repos", "project"), "~/Repos/project"},
		{filepath.Join(home, "Repos", "project", ".claude"), "~/Repos/project/.claude"},
		{"/other/path", "/other/path"},
		{home, "~"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			assert.Equal(t, tt.expected, shortenPath(tt.input))
		})
	}
}
