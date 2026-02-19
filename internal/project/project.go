package project

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"

	"go.yaml.in/yaml/v3"
)

var ErrNoProjectConfig = errors.New("no .claude-sync.yaml found")

const ConfigFileName = ".claude-sync.yaml"

// ProjectConfig represents .claude/.claude-sync.yaml in a project directory.
type ProjectConfig struct {
	Version       string           `yaml:"version"`
	Profile       string           `yaml:"profile,omitempty"`
	Initialized   string           `yaml:"initialized,omitempty"`
	Declined      bool             `yaml:"declined,omitempty"`
	ProjectedKeys []string         `yaml:"projected_keys,omitempty"`
	Overrides     ProjectOverrides `yaml:"overrides,omitempty"`
}

type ProjectOverrides struct {
	Permissions ProjectPermissionOverrides `yaml:"permissions,omitempty"`
	Hooks       ProjectHookOverrides       `yaml:"hooks,omitempty"`
	ClaudeMD    ProjectClaudeMDOverrides   `yaml:"claude_md,omitempty"`
	MCP         ProjectMCPOverrides        `yaml:"mcp,omitempty"`
}

type ProjectPermissionOverrides struct {
	AddAllow []string `yaml:"add_allow,omitempty"`
	AddDeny  []string `yaml:"add_deny,omitempty"`
}

type ProjectHookOverrides struct {
	Add    map[string]json.RawMessage `yaml:"add,omitempty"`
	Remove []string                   `yaml:"remove,omitempty"`
}

type ProjectClaudeMDOverrides struct {
	Add    []string `yaml:"add,omitempty"`
	Remove []string `yaml:"remove,omitempty"`
}

type ProjectMCPOverrides struct {
	Add    map[string]json.RawMessage `yaml:"add,omitempty"`
	Remove []string                   `yaml:"remove,omitempty"`
}

func configPath(projectDir string) string {
	return filepath.Join(projectDir, ".claude", ConfigFileName)
}

func ReadProjectConfig(projectDir string) (ProjectConfig, error) {
	data, err := os.ReadFile(configPath(projectDir))
	if err != nil {
		if os.IsNotExist(err) {
			return ProjectConfig{}, ErrNoProjectConfig
		}
		return ProjectConfig{}, err
	}
	var cfg ProjectConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return ProjectConfig{}, err
	}
	return cfg, nil
}

func WriteProjectConfig(projectDir string, cfg ProjectConfig) error {
	dir := filepath.Join(projectDir, ".claude")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	return os.WriteFile(configPath(projectDir), data, 0644)
}

// FindProjectRoot walks up from dir looking for .claude/.claude-sync.yaml.
func FindProjectRoot(dir string) (string, error) {
	dir, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(configPath(dir)); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", ErrNoProjectConfig
		}
		dir = parent
	}
}
