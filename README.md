# claude-sync

Sync Claude Code configuration across multiple machines.

## Features

- **Declarative config** - Define plugins, settings, and hooks in YAML
- **Auto-sync** - Pulls latest config on Claude Code startup
- **Plugin intelligence** - Track upstream updates, manage forks, pin versions
- **Profiles** - Compose configurations (base + work/personal)
- **Full history** - Git-backed versioning with rollback support
- **User preferences** - Personal overrides on shared team config

## Quick Start

```bash
# New setup (from your current Claude Code state)
claude-sync init

# Or join existing config
claude-sync join git@github.com:your-org/claude-sync-config.git
```

## Usage

```bash
claude-sync status        # Show current state
claude-sync pull          # Pull latest config
claude-sync push          # Push your changes
claude-sync update        # Apply plugin updates
```

## Documentation

See [docs/plans/2026-01-28-claude-sync-design.md](docs/plans/2026-01-28-claude-sync-design.md) for the full design document.

## Development

```bash
# Build
go build -o bin/claude-sync ./cmd/claude-sync

# Test
go test ./...

# Run locally
./bin/claude-sync status
```

## License

MIT
