#!/bin/bash
# SessionStart hook: pull config, set up per-session state, detect unmanaged projects
# Fail-safe: timeouts, locking, stale cleanup, visible error messages

set -euo pipefail

source "$(dirname "$0")/lib.sh"

# If claude-sync is not set up, exit silently
if [ ! -d "$SYNC_DIR" ]; then
    echo "{}"
    exit 0
fi

CLAUDE_SYNC=$(resolve_claude_sync)
if [ -z "$CLAUDE_SYNC" ]; then
    echo "{}"
    exit 0
fi

# --- Step 1: Clean up stale session dirs (dead PIDs) ---
if [ -d "$SESSIONS_DIR" ]; then
    for session_dir in "$SESSIONS_DIR"/*/; do
        [ -d "$session_dir" ] || continue
        pid=$(basename "$session_dir")
        if ! kill -0 "$pid" 2>/dev/null; then
            rm -rf "$session_dir"
        fi
    done
fi

# --- Step 2: Pull config with lock and timeout ---
pull_ok=true
if acquire_lock; then
    trap 'release_lock' EXIT
    if ! run_with_timeout 15 "$CLAUDE_SYNC" pull --auto; then
        pull_ok=false
    fi
    release_lock
    trap - EXIT
else
    pull_ok=false
    echo "claude-sync: lock wait exceeded (3s) during pull — config may be stale. Run \`claude-sync pull --auto\` to retry." >&2
fi

if [ "$pull_ok" = false ]; then
    echo "claude-sync pull failed or timed out — config may be stale. Run \`claude-sync pull --auto\` to retry." >&2
fi

# --- Step 3: Set up per-session state ---
SESSION_ID=$(get_session_id)

if [ -z "$SESSION_ID" ]; then
    echo "claude-sync: could not determine session ID — change detection disabled for this session." >&2
    echo "{}"
    exit 0
fi

SESSION_DIR="$SESSIONS_DIR/$SESSION_ID"
mkdir -p "$SESSION_DIR"

# Save baseline hash (change detection works for local edits even if pull failed)
if [ -f "$CLAUDE_MD" ]; then
    md5 -q "$CLAUDE_MD" 2>/dev/null > "$SESSION_DIR/hash" \
        || md5sum "$CLAUDE_MD" 2>/dev/null | cut -d' ' -f1 > "$SESSION_DIR/hash"
fi

# --- Step 4: Detect unmanaged project ---
project_root=$(git rev-parse --show-toplevel 2>/dev/null || true)
sync_dir_real=$(cd "$SYNC_DIR" 2>/dev/null && pwd -P)

# Skip if we're in the config repo itself
if [ -n "$project_root" ] && [ -n "$sync_dir_real" ] && \
   [ "$(cd "$project_root" 2>/dev/null && pwd -P)" = "$sync_dir_real" ]; then
    echo "{}"
    exit 0
fi

if [ -n "$project_root" ] && [ ! -f "$project_root/.claude/.claude-sync.yaml" ]; then
    profiles=$(ls "$HOME/.claude-sync/profiles/" 2>/dev/null | sed 's/\.yaml$//' | tr '\n' ', ' | sed 's/,$//')
    MSG="ACTION REQUIRED: This project ($project_root) is not managed by claude-sync. Hooks and permissions may be missing. Before doing any work, ask the user to run: claude-sync project init --profile <name>. Available profiles: ${profiles:-none}. To skip: claude-sync project init --decline."

    # Output both structured JSON (for Claude's context) and plain text (for display)
    cat <<EOF
{
  "hookSpecificOutput": {
    "hookEventName": "SessionStart",
    "additionalContext": "$MSG"
  }
}
EOF
    exit 0
fi

echo "{}"
exit 0
