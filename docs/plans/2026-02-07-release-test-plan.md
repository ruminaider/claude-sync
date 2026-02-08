# claude-sync v0.1.0 Release Test Plan

**Goal:** Validate all features work end-to-end across 2 machines before tagging v0.1.0.

**Prerequisites:**
- Machine A: your primary dev machine (has Claude Code with plugins installed)
- Machine B: a second machine (or VM/container with Claude Code installed)
- A private GitHub/GitLab repo for the sync remote (or create one during testing)
- claude-sync binary installed on both machines (`make build && make install`)

**Notation:** `[A]` = run on Machine A, `[B]` = run on Machine B

---

## Phase 1: Clean Slate Setup

### 1.1 Remove any existing sync state

```bash
# [A] and [B]
rm -rf ~/.claude-sync
```

### 1.2 Verify binary works

```bash
# [A] and [B]
claude-sync version
# Expected: claude-sync 0.1.0-dev (or whatever version you built)

claude-sync --help
# Expected: All commands listed: init, join, pull, push, status, setup,
#           migrate, pin, unpin, fork, unfork, update, version

claude-sync status
# Expected: Error "claude-sync not initialized"
```

- [ ] version shows correct string on both machines
- [ ] --help lists all 13 commands (init, join, pull, push, status, setup, migrate, pin, unpin, fork, unfork, update, version)
- [ ] status before init gives clear error message

---

## Phase 2: Init + Remote Setup (Machine A)

### 2.1 Initialize from current Claude Code state

```bash
# [A]
claude-sync init
```

- [ ] Creates ~/.claude-sync/ directory
- [ ] Creates config.yaml with `version: "2.0.0"`
- [ ] Plugins listed under `upstream:` category
- [ ] Settings and hooks extracted (if any)
- [ ] Git repo initialized with initial commit

### 2.2 Inspect the generated config

```bash
# [A]
cat ~/.claude-sync/config.yaml
```

- [ ] Version is "2.0.0"
- [ ] All your installed plugins appear under `plugins.upstream`
- [ ] `enabledPlugins`, `statusLine`, `permissions` are NOT in settings
- [ ] Hooks section only contains user-defined hooks (not plugin-provided ones)

### 2.3 Check status after init

```bash
# [A]
claude-sync status
claude-sync status --json
```

- [ ] All plugins show as synced (green checkmarks)
- [ ] No missing or untracked plugins
- [ ] --json outputs valid JSON with `config_version: "2.0.0"`
- [ ] JSON contains `upstream_synced` array with your plugins

### 2.4 Add remote and push

```bash
# [A]
cd ~/.claude-sync
git remote add origin <your-private-repo-url>
git push -u origin HEAD
```

- [ ] Push succeeds to remote

### 2.5 Verify init is idempotent-safe

```bash
# [A]
claude-sync init
# Expected: Error (already initialized)
```

- [ ] Second init fails with clear error

---

## Phase 3: Join + Pull (Machine B)

### 3.1 Join from remote

```bash
# [B]
claude-sync join <your-private-repo-url>
```

- [ ] Clones repo to ~/.claude-sync/
- [ ] No errors

### 3.2 Check status on Machine B

```bash
# [B]
claude-sync status
```

- [ ] Shows plugins from Machine A's config
- [ ] Some may show as "not installed" (expected if Machine B has fewer plugins)
- [ ] Any locally-installed plugins not in config show as "untracked"

### 3.3 Pull to sync

```bash
# [B]
claude-sync pull
```

- [ ] Attempts to install missing plugins via `claude plugin install`
- [ ] Reports success/failure for each plugin
- [ ] Retries failed plugins once
- [ ] Final status shows what was installed

### 3.4 Verify status after pull

```bash
# [B]
claude-sync status
```

- [ ] Previously missing plugins now show as synced
- [ ] Any install failures still show as missing

### 3.5 Verify join is idempotent-safe

```bash
# [B]
claude-sync join <url>
# Expected: Error (already initialized)
```

- [ ] Second join fails with clear error

---

## Phase 4: Push Flow (Machine A adds a plugin)

### 4.1 Install a new plugin manually

```bash
# [A] — Install any plugin you don't already have via Claude Code
claude plugin install <some-plugin>@<marketplace>
```

### 4.2 Push with interactive selection

```bash
# [A]
claude-sync push
```

- [ ] Shows "Scanning local state..."
- [ ] Detects the new plugin
- [ ] Shows interactive multi-select with new plugin pre-selected
- [ ] Can toggle selection with space bar
- [ ] Pressing enter commits selected changes
- [ ] Auto-generates commit message like "Add some-plugin"
- [ ] Pushes to remote

### 4.3 Push with --all flag

```bash
# [A] — Install another plugin first, then:
claude-sync push --all
```

- [ ] Skips interactive selection
- [ ] Pushes all changes

### 4.4 Push with custom message

```bash
# [A] — Install another plugin first, then:
claude-sync push -m "Add my favorite plugin"
```

- [ ] Uses provided commit message

### 4.5 Push with no changes

```bash
# [A]
claude-sync push
# Expected: "Nothing to push"
```

- [ ] No-op when nothing changed

### 4.6 Verify on Machine B

```bash
# [B]
claude-sync pull
claude-sync status
```

- [ ] Machine B picks up the new plugin(s) from Machine A
- [ ] Status shows them as synced (or installed)

---

## Phase 5: Pin / Unpin

### 5.1 Pin a plugin

```bash
# [A]
claude-sync pin <plugin>@<marketplace> 1.0.0
```

- [ ] Plugin moves from upstream to pinned in config.yaml
- [ ] Config shows `- <plugin>@<marketplace>: "1.0.0"` under pinned
- [ ] Creates git commit

### 5.2 Verify status shows pinned

```bash
# [A]
claude-sync status
claude-sync status --json
```

- [ ] Pinned section shows the plugin with version
- [ ] Upstream section no longer contains it
- [ ] JSON output has `pinned_synced` with the plugin

### 5.3 Sync pin to Machine B

```bash
# [A]
cd ~/.claude-sync && git push

# [B]
claude-sync pull
claude-sync status
```

- [ ] Machine B sees the plugin as pinned
- [ ] Status shows correct pinned version

### 5.4 Unpin the plugin

```bash
# [A]
claude-sync unpin <plugin>@<marketplace>
```

- [ ] Plugin moves back to upstream
- [ ] Pinned section is empty (or plugin removed)
- [ ] Creates git commit

### 5.5 Verify after unpin

```bash
# [A]
claude-sync status
cat ~/.claude-sync/config.yaml
```

- [ ] Plugin back in upstream list
- [ ] No pinned entries for that plugin

---

## Phase 6: Fork / Unfork

### 6.1 Fork a plugin

```bash
# [A]
claude-sync fork <plugin>@<marketplace>
```

- [ ] Creates ~/.claude-sync/plugins/<plugin-name>/ directory
- [ ] Directory contains .claude-plugin/plugin.json
- [ ] Plugin moves to forked category in config.yaml
- [ ] Creates git commit

### 6.2 Inspect forked files

```bash
# [A]
ls ~/.claude-sync/plugins/
ls ~/.claude-sync/plugins/<plugin-name>/.claude-plugin/
cat ~/.claude-sync/config.yaml | grep -A5 forked
```

- [ ] Plugin files are copied
- [ ] plugin.json manifest exists
- [ ] Config lists plugin under forked

### 6.3 Edit the forked plugin

```bash
# [A] — Make a small edit to the forked plugin (e.g., change description in plugin.json)
# Then push:
cd ~/.claude-sync
git add -A && git commit -m "Customize forked plugin"
git push
```

### 6.4 Sync fork to Machine B

```bash
# [B]
claude-sync pull
```

- [ ] Forked plugin files synced to Machine B's ~/.claude-sync/plugins/
- [ ] Local marketplace registered in known_marketplaces.json
- [ ] Status shows forked plugin

### 6.5 Verify local marketplace

```bash
# [B]
cat ~/.claude/plugins/known_marketplaces.json | grep claude-sync-forks
```

- [ ] claude-sync-forks marketplace entry exists
- [ ] Points to ~/.claude-sync/plugins/ directory

### 6.6 Unfork the plugin

```bash
# [A]
claude-sync unfork <plugin-name> --marketplace <marketplace>
```

- [ ] Removes ~/.claude-sync/plugins/<plugin-name>/ directory
- [ ] Plugin moves back to upstream in config.yaml
- [ ] Creates git commit

---

## Phase 7: Update Command

### 7.1 Check for updates

```bash
# [A]
claude-sync update
```

- [ ] Reports upstream plugins (reinstalls for latest)
- [ ] Reports pinned plugins (shows current version, not auto-updated)
- [ ] Reports forked plugins (reinstalls from sync repo)

### 7.2 Quiet mode

```bash
# [A]
claude-sync update --quiet
```

- [ ] Suppresses output but still performs updates

---

## Phase 8: Migration (v1 → v2)

> **Note:** To test this, you need a v1 config. You can create one manually.

### 8.1 Create a v1 config manually

```bash
# [A] or a test machine
rm -rf ~/.claude-sync
claude-sync init
# Edit config.yaml to downgrade to v1 format:
```

Write this to ~/.claude-sync/config.yaml:
```yaml
version: "1.0.0"

plugins:
  - context7@claude-plugins-official
  - beads@beads-marketplace
  - episodic-memory@superpowers-marketplace
```

```bash
cd ~/.claude-sync && git add -A && git commit -m "Downgrade to v1 for testing"
```

### 8.2 Run migration

```bash
claude-sync migrate
```

- [ ] Detects v1 config
- [ ] Lists all plugins for categorization
- [ ] Prompts for each plugin: upstream / pinned / forked
- [ ] For pinned: prompts for version
- [ ] Writes v2 config
- [ ] Creates git commit "Migrate config to v2"

### 8.3 Verify migrated config

```bash
cat ~/.claude-sync/config.yaml
```

- [ ] Version is "2.0.0"
- [ ] Plugins categorized as selected
- [ ] Settings and hooks preserved

### 8.4 Migration on already-v2 config

```bash
claude-sync migrate
# Expected: "Already on v2" or similar skip message
```

- [ ] No-ops on v2 config

---

## Phase 9: Shell Alias Integration

### 9.1 Setup command

```bash
# [A]
claude-sync setup
```

- [ ] Detects your shell (bash/zsh/fish)
- [ ] Shows correct alias command
- [ ] Shows correct rc file path

### 9.2 Install the alias

```bash
# [A] — Add the alias to your shell rc file as instructed
# Then source it
source ~/.zshrc  # or ~/.bashrc
```

### 9.3 Test the alias

```bash
# [A]
# The 'claude' command should now run pull first
# You can verify by checking timestamps or adding a --verbose flag
type claude
# Should show it's an alias
```

- [ ] `claude` command triggers pull before launching Claude Code
- [ ] Pull output is suppressed (quiet mode)
- [ ] Claude Code launches normally after pull

---

## Phase 10: SessionStart Hook

### 10.1 Verify hook exists in plugin

```bash
# [A]
ls ~/.claude-sync/../  # Check if plugin is installed
cat ~/.claude/plugins/cache/*/claude-sync/*/hooks/session-start.sh 2>/dev/null || echo "Hook not found in cache"
```

### 10.2 Test hook behavior

```bash
# [A] — Start a new Claude Code session
claude
# On session start, the hook should:
# - Check for config changes (git fetch + compare)
# - Show notification if changes pending
# - Be silent if no changes
```

- [ ] Hook runs on session start
- [ ] Shows notification when remote has changes
- [ ] Silent when no remote changes
- [ ] Respects 30-second debounce (open 2 sessions quickly)
- [ ] Fails silently on network error

---

## Phase 11: Edge Cases

### 11.1 Network offline

```bash
# Disconnect from network, then:
claude-sync pull
# Expected: Warning about git pull failure, but uses local config

claude-sync push
# Expected: Commits locally, fails on push with clear error
```

- [ ] Pull works offline with local config
- [ ] Push fails gracefully offline

### 11.2 Concurrent sessions

```bash
# Open 3 terminal windows quickly and run in each:
claude-sync pull
# Second and third should be fast (debounce or fast git)
```

- [ ] No file locking errors
- [ ] No git corruption

### 11.3 Empty config edge case

```bash
# Create config with no plugins:
echo 'version: "2.0.0"' > ~/.claude-sync/config.yaml
echo 'plugins:' >> ~/.claude-sync/config.yaml
echo '  upstream: []' >> ~/.claude-sync/config.yaml
claude-sync status
```

- [ ] Handles empty plugin lists gracefully

### 11.4 Plugin install failure

```bash
# Add a nonexistent plugin to config manually:
# Under upstream, add: "nonexistent-plugin@fake-marketplace"
claude-sync pull
```

- [ ] Reports failure for that plugin
- [ ] Continues with other plugins
- [ ] Retries once
- [ ] Clear error message

---

## Phase 12: Performance Checks

```bash
# [A]
time claude-sync status          # Target: < 500ms
time claude-sync pull            # Target: < 200ms (no changes)
```

- [ ] Status completes in < 500ms
- [ ] Pull with no changes completes in < 200ms (excluding network)

---

## Phase 13: Automated Tests (CI sanity check)

```bash
# Run the full test suite one final time
make test
go test -tags=integration ./tests/ -v
```

- [ ] All unit tests pass (80+)
- [ ] Both integration tests pass (TestFullWorkflow, TestV2Workflow)
- [ ] Build succeeds on target platforms (`make cross-compile` if available)

---

## Release Checklist

After all phases pass:

- [ ] All Phase 1-13 checkboxes checked
- [ ] Update version string in `cmd/claude-sync/main.go` from `0.1.0-dev` to `0.1.0`
- [ ] `make build && make test`
- [ ] Tag release: `git tag v0.1.0 && git push --tags`
- [ ] Build release binaries for darwin-arm64, darwin-amd64, linux-amd64
- [ ] Write release notes summarizing features
- [ ] Publish

---

## Quick Two-Machine Smoke Test (15-minute version)

If you're short on time, this covers the critical path:

1. **[A]** `claude-sync init` — verify config.yaml created
2. **[A]** Add remote, `git push`
3. **[B]** `claude-sync join <url>` — verify cloned
4. **[B]** `claude-sync status` — verify missing plugins shown
5. **[B]** `claude-sync pull` — verify plugins installed
6. **[A]** Install a new plugin, `claude-sync push --all`
7. **[B]** `claude-sync pull` — verify new plugin arrives
8. **[A]** `claude-sync pin <plugin> 1.0.0` + push
9. **[B]** `claude-sync pull && claude-sync status` — verify pinned shows
10. **[A]** `claude-sync unpin <plugin>` + push
11. **[B]** `claude-sync pull && claude-sync status` — verify back to upstream
12. **[A]** `claude-sync status --json` — verify JSON output
13. Both: `make test` — all green
