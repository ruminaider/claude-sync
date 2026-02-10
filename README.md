# claude-sync

Sync Claude Code configuration across multiple machines.

## Features

- **Declarative config** - Define plugins, settings, and hooks in YAML
- **Auto-sync** - Pulls latest config on Claude Code startup
- **Plugin intelligence** - Track upstream updates, manage forks, pin versions
- **Profiles** - Compose configurations (base + work/personal)
- **Full history** - Git-backed versioning with rollback support
- **User preferences** - Personal overrides on shared team config

## Installation

### Quick install (recommended)

```bash
curl -fsSL https://raw.githubusercontent.com/ruminaider/claude-sync/main/install.sh | sh
```

This downloads the latest release binary for your platform (macOS/Linux, amd64/arm64) and installs it to `~/.local/bin`.

To install a specific version:

```bash
curl -fsSL https://raw.githubusercontent.com/ruminaider/claude-sync/main/install.sh | sh -s v0.1.15
```

### Manual download

Download a binary for your platform from the [Releases](https://github.com/ruminaider/claude-sync/releases) page:

```bash
chmod +x claude-sync-*
mv claude-sync-* ~/.local/bin/claude-sync
```

### Build from source

Requires Go 1.23+.

```bash
git clone https://github.com/ruminaider/claude-sync.git
cd claude-sync
make install
```

## Quick Start

```bash
# New setup (from your current Claude Code state)
claude-sync init

# Or join an existing shared config
claude-sync join git@github.com:your-org/claude-sync-config.git
```

Both commands install a Claude Code plugin that automatically checks for config updates at the start of each session and provides the `/sync` slash command.

## Usage

```bash
claude-sync status        # Show current state
claude-sync pull          # Pull latest config
claude-sync push          # Push your changes
claude-sync update        # Apply plugin updates
```

### Inside Claude Code

The bundled plugin gives you:

- **Auto-sync on startup** - A `SessionStart` hook fetches remote changes and notifies you when updates are pending
- **`/sync` command** - Run `/sync status`, `/sync pull`, or `/sync apply` directly in Claude Code

## Plugin Forking

When you initialize or join a shared config, claude-sync detects **non-portable plugins** — plugins installed from a local path or marketplace that isn't available on other machines. These are automatically forked:

1. The plugin directory is copied to `~/.claude-sync/plugins/<name>/`
2. The config records it in the `forked:` category instead of `upstream:`
3. A **local marketplace** (`claude-sync-forks`) is registered in `~/.claude/plugins/known_marketplaces.json` so Claude Code can discover the forked plugins

You can also manually fork and unfork plugins:

```bash
claude-sync fork <name>@<marketplace>    # Fork a plugin for local customization
claude-sync unfork <name>@<marketplace>  # Return to upstream tracking
```

### Cleanup

The local marketplace entry is automatically removed when no forked plugins remain — for example, after unforking the last plugin, switching to a profile with no forks, or running `pull` against a config with no forks.

If you see a **"Failed to load marketplace 'claude-sync-forks'"** warning from Claude Code, run `claude-sync pull` to clean up the stale entry, or manually remove the `claude-sync-forks` key from `~/.claude/plugins/known_marketplaces.json`.

## Supported Platforms

| OS    | Architecture |
|-------|-------------|
| macOS | arm64 (Apple Silicon), amd64 (Intel) |
| Linux | amd64, arm64 |

## Documentation

See [docs/plans/2026-01-28-claude-sync-design.md](docs/plans/2026-01-28-claude-sync-design.md) for the full design document.

## Development

```bash
make build       # Build binary
make test        # Run tests
make install     # Build and install locally
```

## License

MIT
