#!/bin/bash
set -e

SCRIPT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
ARCH=$(uname -m)
OS=$(uname -s | tr '[:upper:]' '[:lower:]')

case "$ARCH" in
  x86_64) ARCH="amd64" ;;
  aarch64|arm64) ARCH="arm64" ;;
esac

CLAUDE_SYNC="${SCRIPT_DIR}/bin/claude-sync-${OS}-${ARCH}"

if [ ! -x "$CLAUDE_SYNC" ]; then
  CLAUDE_SYNC=$(command -v claude-sync 2>/dev/null || echo "")
fi

if [ -z "$CLAUDE_SYNC" ]; then
  echo "{}"
  exit 0
fi

if [ ! -d "$HOME/.claude-sync" ]; then
  echo "{}"
  exit 0
fi

SYNC_DIR="$HOME/.claude-sync"

LAST_FETCH_FILE="${SYNC_DIR}/.last_fetch"
FETCH_INTERVAL=30

now=$(date +%s)
last_fetch=$(cat "$LAST_FETCH_FILE" 2>/dev/null || echo 0)

if [ $((now - last_fetch)) -gt $FETCH_INTERVAL ]; then
  cd "$SYNC_DIR"
  timeout 2 git fetch --quiet 2>/dev/null || true
  echo "$now" > "$LAST_FETCH_FILE"
fi

cd "$SYNC_DIR"
LOCAL=$(git rev-parse HEAD 2>/dev/null || echo "")
REMOTE=$(git rev-parse @{u} 2>/dev/null || echo "$LOCAL")

CONFIG_CHANGES=false
if [ -n "$LOCAL" ] && [ "$LOCAL" != "$REMOTE" ]; then
  CONFIG_CHANGES=true
fi

if [ "$CONFIG_CHANGES" = true ]; then
  MSG="claude-sync: Config changes pending. Run '/sync status' for details. Restart session to apply."

  cat <<EOF
{
  "hookSpecificOutput": {
    "hookEventName": "SessionStart",
    "additionalContext": "$MSG"
  }
}
EOF
else
  echo "{}"
fi
