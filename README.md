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
