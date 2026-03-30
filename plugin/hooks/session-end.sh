#!/bin/bash
# SessionEnd hook: push config changes, clean up per-session state
# Fail-safe: timeouts, locking, visible error messages, always cleans up

set -euo pipefail

source "$(dirname "$0")/lib.sh"

# If claude-sync is not set up, exit silently
if [ ! -d "$SYNC_DIR" ]; then
    exit 0
fi

CLAUDE_SYNC=$(resolve_claude_sync)
if [ -z "$CLAUDE_SYNC" ]; then
    exit 0
fi

SESSION_ID=$(get_session_id)
SESSION_DIR="$SESSIONS_DIR/$SESSION_ID"

# --- Step 1: Push config with lock and timeout ---
push_ok=true
if acquire_lock; then
    trap 'release_lock' EXIT
    if ! run_with_timeout 15 "$CLAUDE_SYNC" push --auto --quiet; then
        push_ok=false
    fi
    release_lock
    trap - EXIT
else
    push_ok=false
    echo "claude-sync: lock wait exceeded (3s) during push — changes may not have synced. Run \`claude-sync push --auto\` to retry." >&2
fi

if [ "$push_ok" = false ]; then
    echo "claude-sync push failed or timed out — changes may not have synced to remote. Run \`claude-sync push --auto\` to retry." >&2
fi

# --- Step 2: Clean up this session's dir (always, regardless of push result) ---
if [ -n "$SESSION_ID" ] && [ -d "$SESSION_DIR" ]; then
    rm -rf "$SESSION_DIR"
fi

exit 0
