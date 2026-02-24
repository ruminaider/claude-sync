package commands

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode"

	"github.com/ruminaider/claude-sync/internal/claudecode"
	"github.com/ruminaider/claude-sync/internal/config"
	"github.com/ruminaider/claude-sync/internal/git"
	"github.com/ruminaider/claude-sync/internal/profiles"
)

// DetectedSecret represents a secret value found in an MCP server's env config.
type DetectedSecret struct {
	ServerName string // which MCP server contains this secret
	EnvKey     string // key in the env map (e.g., "RENDER_API_KEY")
	Value      string // the actual secret value
	Reason     string // why it was flagged (e.g., "key matches *_API_KEY")
}

// MCPImportScanResult holds the result of scanning an MCP config file for import.
type MCPImportScanResult struct {
	SourcePath string
	Servers    map[string]json.RawMessage
	Secrets    []DetectedSecret
}

// DetectMCPSecrets scans in-memory MCP server configs for secrets.
// Use this when configs are already loaded (vs MCPImportScan which reads from file).
func DetectMCPSecrets(servers map[string]json.RawMessage) []DetectedSecret {
	var secrets []DetectedSecret
	for name, raw := range servers {
		secrets = append(secrets, detectSecrets(name, raw)...)
	}
	sort.Slice(secrets, func(i, j int) bool {
		if secrets[i].ServerName != secrets[j].ServerName {
			return secrets[i].ServerName < secrets[j].ServerName
		}
		return secrets[i].EnvKey < secrets[j].EnvKey
	})
	return secrets
}

// MCPImportScan reads an .mcp.json file and detects secrets in env values.
func MCPImportScan(sourcePath string) (*MCPImportScanResult, error) {
	servers, err := claudecode.ReadMCPConfigFile(sourcePath)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", sourcePath, err)
	}
	if len(servers) == 0 {
		return nil, fmt.Errorf("no MCP servers found in %s", sourcePath)
	}

	var secrets []DetectedSecret
	for name, raw := range servers {
		secrets = append(secrets, detectSecrets(name, raw)...)
	}

	sort.Slice(secrets, func(i, j int) bool {
		if secrets[i].ServerName != secrets[j].ServerName {
			return secrets[i].ServerName < secrets[j].ServerName
		}
		return secrets[i].EnvKey < secrets[j].EnvKey
	})

	return &MCPImportScanResult{
		SourcePath: sourcePath,
		Servers:    servers,
		Secrets:    secrets,
	}, nil
}

// detectSecrets inspects a single MCP server config for secret values.
func detectSecrets(serverName string, raw json.RawMessage) []DetectedSecret {
	var cfg struct {
		Env map[string]string `json:"env"`
	}
	if err := json.Unmarshal(raw, &cfg); err != nil || len(cfg.Env) == 0 {
		return nil
	}

	var secrets []DetectedSecret
	for key, value := range cfg.Env {
		// Skip values already templated as ${VAR}
		if isTemplated(value) {
			continue
		}

		if reason := classifySecret(key, value); reason != "" {
			secrets = append(secrets, DetectedSecret{
				ServerName: serverName,
				EnvKey:     key,
				Value:      value,
				Reason:     reason,
			})
		}
	}
	return secrets
}

// classifySecret returns a reason string if the key/value pair looks like a secret, or "" if not.
func classifySecret(key, value string) string {
	if reason := isSecretKey(key); reason != "" {
		return reason
	}
	if reason := isSecretValue(value); reason != "" {
		return reason
	}
	return ""
}

// isSecretKey checks if the env key name matches common secret patterns.
func isSecretKey(key string) string {
	upper := strings.ToUpper(key)
	patterns := []string{"_KEY", "_TOKEN", "_SECRET", "_PASSWORD", "_API_KEY", "_APIKEY"}
	for _, suffix := range patterns {
		if strings.HasSuffix(upper, suffix) {
			return fmt.Sprintf("key matches *%s", suffix)
		}
	}
	return ""
}

// Known secret value prefixes from common services.
var secretPrefixes = []string{
	"sk-",    // OpenAI, Stripe
	"rnd_",   // Render
	"NRAK-",  // New Relic
	"xoxc-",  // Slack client token
	"xoxd-",  // Slack D token
	"xoxb-",  // Slack bot token
	"xoxp-",  // Slack user token
	"shpat_", // Shopify
	"pa-",    // PlanetScale
	"live_",  // various payment providers
}

// isSecretValue checks if the value matches known secret prefixes or looks like a long credential.
func isSecretValue(value string) string {
	for _, prefix := range secretPrefixes {
		if strings.HasPrefix(value, prefix) {
			return fmt.Sprintf("value matches %s* prefix", prefix)
		}
	}

	// Long alphanumeric strings (32+ chars, 80%+ alnum) are likely secrets.
	if len(value) >= 32 {
		alnum := 0
		for _, r := range value {
			if unicode.IsLetter(r) || unicode.IsDigit(r) {
				alnum++
			}
		}
		if float64(alnum)/float64(len(value)) >= 0.8 {
			return "long alphanumeric string (likely credential)"
		}
	}

	return ""
}

// isTemplated returns true if the value is already a ${VAR} reference.
func isTemplated(value string) bool {
	return strings.HasPrefix(value, "${") && strings.HasSuffix(value, "}")
}

// envVarRefPattern matches ${VAR_NAME} references in strings.
var envVarRefPattern = regexp.MustCompile(`\$\{([^}]+)\}`)

// ReplaceSecrets replaces detected secret values in MCP server configs with ${ENV_KEY} references.
func ReplaceSecrets(servers map[string]json.RawMessage, secrets []DetectedSecret) map[string]json.RawMessage {
	// Build a lookup: serverName -> envKey -> true
	secretSet := make(map[string]map[string]bool)
	for _, s := range secrets {
		if secretSet[s.ServerName] == nil {
			secretSet[s.ServerName] = make(map[string]bool)
		}
		secretSet[s.ServerName][s.EnvKey] = true
	}

	result := make(map[string]json.RawMessage, len(servers))
	for name, raw := range servers {
		keys, ok := secretSet[name]
		if !ok {
			result[name] = raw
			continue
		}

		var cfg map[string]any
		if err := json.Unmarshal(raw, &cfg); err != nil {
			result[name] = raw
			continue
		}

		envRaw, ok := cfg["env"]
		if !ok {
			result[name] = raw
			continue
		}
		env, ok := envRaw.(map[string]any)
		if !ok {
			result[name] = raw
			continue
		}

		for key := range keys {
			env[key] = "${" + key + "}"
		}
		cfg["env"] = env

		data, err := json.Marshal(cfg)
		if err != nil {
			result[name] = raw
			continue
		}
		result[name] = json.RawMessage(data)
	}

	return result
}

// MCPImportOptions configures the import operation.
type MCPImportOptions struct {
	SyncDir     string
	Servers     map[string]json.RawMessage // after secret replacement
	Profile     string                     // "" = base config
	ProjectPath string                     // hint for pull-side routing
}

// MCPImportResult holds the result of an MCP import operation.
type MCPImportResult struct {
	Imported        []string
	SecretsReplaced int
	TargetProfile   string
	ProjectPath     string
}

// MCPImport merges the given MCP servers into the sync config and commits.
func MCPImport(opts MCPImportOptions) (*MCPImportResult, error) {
	cfgPath := filepath.Join(opts.SyncDir, "config.yaml")
	cfgData, err := os.ReadFile(cfgPath)
	if err != nil {
		return nil, fmt.Errorf("reading config.yaml: %w", err)
	}
	cfg, err := config.Parse(cfgData)
	if err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}

	var imported []string

	if opts.Profile == "" {
		// Merge into base config.
		if cfg.MCP == nil {
			cfg.MCP = make(map[string]json.RawMessage)
		}
		for name, raw := range opts.Servers {
			cfg.MCP[name] = raw
			imported = append(imported, name)
		}
	} else {
		// Merge into profile.
		profile, err := profiles.ReadProfile(opts.SyncDir, opts.Profile)
		if err != nil {
			return nil, fmt.Errorf("reading profile %q: %w", opts.Profile, err)
		}

		if profile.MCP.Add == nil {
			profile.MCP.Add = make(map[string]json.RawMessage)
		}
		for name, raw := range opts.Servers {
			profile.MCP.Add[name] = raw
			imported = append(imported, name)
		}

		profileData, err := profiles.MarshalProfile(profile)
		if err != nil {
			return nil, fmt.Errorf("marshaling profile: %w", err)
		}
		profilePath := filepath.Join(opts.SyncDir, "profiles", opts.Profile+".yaml")
		if err := os.WriteFile(profilePath, profileData, 0644); err != nil {
			return nil, fmt.Errorf("writing profile: %w", err)
		}

		// Stage profile file.
		profileRelPath := filepath.Join("profiles", opts.Profile+".yaml")
		_ = git.Add(opts.SyncDir, profileRelPath)
	}

	// Update MCPMeta with project path hints.
	if opts.ProjectPath != "" {
		if cfg.MCPMeta == nil {
			cfg.MCPMeta = make(map[string]config.MCPServerMeta)
		}
		for name := range opts.Servers {
			cfg.MCPMeta[name] = config.MCPServerMeta{
				SourceProject: opts.ProjectPath,
			}
		}
	}

	sort.Strings(imported)

	// Write config.
	newData, err := config.Marshal(cfg)
	if err != nil {
		return nil, fmt.Errorf("marshaling config: %w", err)
	}
	if err := os.WriteFile(cfgPath, newData, 0644); err != nil {
		return nil, fmt.Errorf("writing config: %w", err)
	}

	// Stage and commit.
	if err := git.Add(opts.SyncDir, "config.yaml"); err != nil {
		return nil, fmt.Errorf("staging config: %w", err)
	}
	commitMsg := "Import MCP servers: " + strings.Join(imported, ", ")
	if opts.Profile != "" {
		commitMsg += " (profile: " + opts.Profile + ")"
	}
	if err := git.Commit(opts.SyncDir, commitMsg); err != nil {
		return nil, fmt.Errorf("committing: %w", err)
	}

	return &MCPImportResult{
		Imported:      imported,
		TargetProfile: opts.Profile,
		ProjectPath:   opts.ProjectPath,
	}, nil
}

// ResolveMCPEnvVars scans MCP server configs for ${VAR} references and resolves
// them from the environment. Returns resolved configs and a list of warnings
// for any unresolved variables.
func ResolveMCPEnvVars(servers map[string]json.RawMessage) (map[string]json.RawMessage, []string) {
	if len(servers) == 0 {
		return servers, nil
	}

	var warnings []string
	result := make(map[string]json.RawMessage, len(servers))

	for name, raw := range servers {
		var cfg map[string]any
		if err := json.Unmarshal(raw, &cfg); err != nil {
			result[name] = raw
			continue
		}

		envRaw, ok := cfg["env"]
		if !ok {
			result[name] = raw
			continue
		}
		env, ok := envRaw.(map[string]any)
		if !ok {
			result[name] = raw
			continue
		}

		changed := false
		for key, val := range env {
			strVal, ok := val.(string)
			if !ok {
				continue
			}
			resolved := envVarRefPattern.ReplaceAllStringFunc(strVal, func(match string) string {
				varName := match[2 : len(match)-1] // strip ${ and }
				if envVal, ok := os.LookupEnv(varName); ok {
					changed = true
					return envVal
				}
				warnings = append(warnings, fmt.Sprintf("%s: env var %s not set (used by %s)", name, varName, key))
				return match // leave unresolved
			})
			env[key] = resolved
		}

		if !changed {
			result[name] = raw
			continue
		}

		cfg["env"] = env
		data, err := json.Marshal(cfg)
		if err != nil {
			result[name] = raw
			continue
		}
		result[name] = json.RawMessage(data)
	}

	sort.Strings(warnings)
	return result, warnings
}
