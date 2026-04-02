package commands

import "testing"

func TestBuildAssetURL(t *testing.T) {
	tests := []struct {
		version, goos, goarch string
		want                  string
	}{
		{
			"0.12.0", "darwin", "arm64",
			"https://github.com/ruminaider/claude-sync/releases/download/v0.12.0/claude-sync-darwin-arm64",
		},
		{
			"0.12.0", "linux", "amd64",
			"https://github.com/ruminaider/claude-sync/releases/download/v0.12.0/claude-sync-linux-amd64",
		},
	}
	for _, tt := range tests {
		t.Run(tt.goos+"_"+tt.goarch, func(t *testing.T) {
			got := buildAssetURL(tt.version, tt.goos, tt.goarch)
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}
