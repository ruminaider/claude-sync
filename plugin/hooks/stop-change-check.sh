#!/bin/bash
# Stop hook: detect config changes using per-session state, ask user before syncing
# Pure file operations only — does NOT call claude-sync (zero memory/CPU risk)

source "$(dirname "$0")/lib.sh"

SESSION_ID=$(get_session_id)

# If we can't determine session ID, skip (no crash, no block)
if [ -z "$SESSION_ID" ]; then
    exit 0
fi

SESSION_DIR="$SESSIONS_DIR/$SESSION_ID"
HASH_FILE="$SESSION_DIR/hash"
ASKED_MARKER="$SESSION_DIR/asked"

# If we already asked this session, don't ask again
if [ -f "$ASKED_MARKER" ]; then
    exit 0
fi

# If no baseline hash exists (session-start didn't run or failed), skip
if [ ! -f "$HASH_FILE" ]; then
    exit 0
fi

# Compare current CLAUDE.md hash against baseline from session start
CURRENT_HASH=$(md5 -q "$CLAUDE_MD" 2>/dev/null || md5sum "$CLAUDE_MD" 2>/dev/null | cut -d' ' -f1)
LAST_HASH=$(cat "$HASH_FILE" 2>/dev/null)

if [ "$CURRENT_HASH" = "$LAST_HASH" ]; then
    # No changes — approve stop silently
    exit 0
fi

# Changes detected — mark as asked so we don't re-ask, then block stop
touch "$ASKED_MARKER"

cat >&2 <<EOF
{"decision":"block","reason":"Your Claude config files (CLAUDE.md) have changed since the last sync. Before stopping, ask the user: 'Your Claude config has changed — would you like to sync these changes with claude-sync?' If they say yes, run: claude-sync auto-commit --if-changed && md5 -q ~/.claude/CLAUDE.md > ~/.claude-sync/sessions/${SESSION_ID}/hash. If they say no, acknowledge and stop normally."}
EOF
exit 2
