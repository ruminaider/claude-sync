#!/bin/bash
# SessionEnd hook: push config changes, clean up per-session state
# Fail-safe: timeouts, locking, visible error messages, always cleans up

set -euo pipefail

source "$(dirname "$0")/lib.sh"
_hook_start=$(debug_time_ms)

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
    run_with_timeout 5 "$CLAUDE_SYNC" push --auto --quiet
    push_status=$?
    if [ $push_status -ne 0 ]; then
        push_ok=false
    fi
    release_lock
    trap - EXIT
else
    push_ok=false
    echo "claude-sync: lock wait exceeded (3s) during push: changes may not have synced. Run \`claude-sync push --auto\` to retry." >&2
fi

if [ "$push_ok" = false ]; then
    if [ "${push_status:-0}" -eq 124 ]; then
        echo "claude-sync push timed out (5s): changes may not have synced to remote. Run \`claude-sync push --auto\` to retry." >&2
    else
        echo "claude-sync push failed: changes may not have synced to remote. Run \`claude-sync push --auto\` to retry." >&2
    fi
fi

# --- Step 2: Clean up this session's dir (always, regardless of push result) ---
if [ -n "$SESSION_ID" ] && [ -d "$SESSION_DIR" ]; then
    rm -rf "$SESSION_DIR"
fi

if [ "${CLAUDE_SYNC_DEBUG:-}" = "1" ]; then
    debug_log "session-end.sh: total $(( $(debug_time_ms) - _hook_start ))ms"
fi
exit 0
