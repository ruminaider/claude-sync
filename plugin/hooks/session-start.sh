#!/bin/bash
# SessionStart hook: pull config, set up per-session state, detect unmanaged projects
# Fail-safe: timeouts, locking, stale cleanup, visible error messages

set -euo pipefail

source "$(dirname "$0")/lib.sh"
_hook_start=$(debug_time_ms)

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
  PULL_STDERR=$(mktemp)
  trap 'release_lock; rm -f "$PULL_STDERR"' EXIT
  run_with_timeout 5 "$CLAUDE_SYNC" pull --auto 2>"$PULL_STDERR" || pull_status=$?
  pull_status=${pull_status:-0}
  PULL_ERR=$(cat "$PULL_STDERR")
  rm -f "$PULL_STDERR"
  release_lock
  trap - EXIT

  if [ $pull_status -ne 0 ]; then
    pull_ok=false
  fi

  # Protocol emitted by cmd/claude-sync/cmd_pull.go; keep in sync.
  UPDATE_LINE=$(echo "$PULL_ERR" | grep "^UPDATE_AVAILABLE:" || true)
  if [ -n "$UPDATE_LINE" ]; then
    CURRENT_VER=$(echo "$UPDATE_LINE" | cut -d: -f2)
    LATEST_VER=$(echo "$UPDATE_LINE" | cut -d: -f3)
    UPDATE_CONTEXT="claude-sync update available: ${CURRENT_VER} -> ${LATEST_VER}. Actively remind the user to run \`claude-sync update\` to get the latest version."
  fi

  # Forward any non-update stderr.
  echo "$PULL_ERR" | grep -v "^UPDATE_AVAILABLE:" >&2 || true
else
  pull_ok=false
  echo "claude-sync: lock wait exceeded (3s) during pull: config may be stale. Run \`claude-sync pull --auto\` to retry." >&2
fi

if [ "$pull_ok" = false ]; then
  if [ "${pull_status:-0}" -eq 124 ]; then
    echo "claude-sync pull timed out (5s): config may be stale. Run \`claude-sync pull --auto\` to retry." >&2
  else
    echo "claude-sync pull failed: config may be stale. Run \`claude-sync pull --auto\` to retry." >&2
  fi
fi

# --- Step 3: Set up per-session state ---
SESSION_ID=$(get_session_id)

if [ -z "$SESSION_ID" ]; then
    echo "claude-sync: could not determine session ID: change detection disabled for this session." >&2
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

# Skip project detection if we're in the config repo itself
in_config_repo=false
if [ -n "$project_root" ] && [ -n "$sync_dir_real" ] && \
   [ "$(cd "$project_root" 2>/dev/null && pwd -P)" = "$sync_dir_real" ]; then
    in_config_repo=true
fi

ADDITIONAL_CONTEXT=""

# Add update notification if available (from Step 2 pull stderr parsing).
if [ -n "${UPDATE_CONTEXT:-}" ]; then
    ADDITIONAL_CONTEXT="$UPDATE_CONTEXT"
fi

# Add unmanaged project warning (skip if in config repo).
if [ "$in_config_repo" = false ] && [ -n "$project_root" ] && \
   [ ! -f "$project_root/.claude/.claude-sync.yaml" ]; then
    profiles=$(ls "$HOME/.claude-sync/profiles/" 2>/dev/null | sed 's/\.yaml$//' | tr '\n' ', ' | sed 's/,$//' || true)
    PROJECT_MSG="ACTION REQUIRED: This project ($project_root) is not managed by claude-sync. Hooks and permissions may be missing. Before doing any work, ask the user to run: claude-sync project init --profile <name>. Available profiles: ${profiles:-none}. To skip: claude-sync project init --decline."
    if [ -n "$ADDITIONAL_CONTEXT" ]; then
        ADDITIONAL_CONTEXT="$ADDITIONAL_CONTEXT\n$PROJECT_MSG"
    else
        ADDITIONAL_CONTEXT="$PROJECT_MSG"
    fi
fi

# --- Final output ---
if [ "${CLAUDE_SYNC_DEBUG:-}" = "1" ]; then
    debug_log "session-start.sh: total $(( $(debug_time_ms) - _hook_start ))ms"
fi

if [ -n "$ADDITIONAL_CONTEXT" ]; then
    cat <<EOF
{
  "hookSpecificOutput": {
    "hookEventName": "SessionStart",
    "additionalContext": "$ADDITIONAL_CONTEXT"
  }
}
EOF
else
    echo "{}"
fi
exit 0
