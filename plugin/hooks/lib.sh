#!/bin/bash
# Shared utilities for claude-sync plugin hooks
# Sourced by session-start.sh, session-end.sh, stop-change-check.sh

SYNC_DIR="$HOME/.claude-sync"
SESSIONS_DIR="$SYNC_DIR/sessions"
CLAUDE_MD="$HOME/.claude/CLAUDE.md"
LOCKDIR="$SYNC_DIR/.lock"

# Debug logging: set CLAUDE_SYNC_DEBUG=1 to enable
debug_time_ms() {
    local ms
    ms=$(date +%s%3N 2>/dev/null)
    if [[ "$ms" == *N ]]; then
        # %N unsupported (stock macOS date): fall back to second precision
        echo "$(date +%s)000"
    else
        echo "$ms"
    fi
}

debug_log() {
    [ "${CLAUDE_SYNC_DEBUG:-}" = "1" ] || return 0
    echo "[claude-sync debug $(debug_time_ms)] $*" >&2
}

# Resolve claude-sync binary: bundled (plugin/bin/) then PATH fallback
resolve_claude_sync() {
    local script_dir
    script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
    local arch os
    arch=$(uname -m)
    os=$(uname -s | tr '[:upper:]' '[:lower:]')
    case "$arch" in
        x86_64) arch="amd64" ;;
        aarch64|arm64) arch="arm64" ;;
    esac
    local bin="${script_dir}/bin/claude-sync-${os}-${arch}"
    if [ -x "$bin" ]; then echo "$bin"; return; fi
    command -v claude-sync 2>/dev/null || echo ""
}

# macOS-compatible timeout (no coreutils dependency)
# Returns 124 on timeout (matching coreutils convention), else the command's exit code.
run_with_timeout() {
    local secs=$1; shift
    "$@" &
    local pid=$!
    ( sleep "$secs" && kill "$pid" 2>/dev/null ) &
    local watchdog=$!
    wait "$pid" 2>/dev/null
    local status=$?
    if ! kill -0 "$watchdog" 2>/dev/null; then
        # Watchdog already exited, meaning it fired (timeout)
        wait "$watchdog" 2>/dev/null
        return 124
    fi
    kill "$watchdog" 2>/dev/null 2>&1; wait "$watchdog" 2>/dev/null
    return $status
}

# mkdir-based lock with stale detection (60s) and blocking wait (3s)
acquire_lock() {
    local waited=0
    local max_wait=15       # 15 iterations × 0.2s = 3s

    while ! mkdir "$LOCKDIR" 2>/dev/null; do
        # Break stale locks: owner dead OR directory older than 60s
        if [ -f "$LOCKDIR/pid" ]; then
            local lock_pid
            lock_pid=$(cat "$LOCKDIR/pid" 2>/dev/null)
            if [ -z "$lock_pid" ] || ! [[ "$lock_pid" =~ ^[0-9]+$ ]]; then
                # Corrupted or empty PID file: break the lock
                rm -rf "$LOCKDIR"
                waited=$((waited + 1))
                continue
            fi
            if ! kill -0 "$lock_pid" 2>/dev/null; then
                rm -rf "$LOCKDIR"
                waited=$((waited + 1))
                continue
            fi
        elif find "$LOCKDIR" -maxdepth 0 -mmin +1 2>/dev/null | grep -q .; then
            rm -rf "$LOCKDIR"
            waited=$((waited + 1))
            continue
        fi
        waited=$((waited + 1))
        if [ $waited -ge $max_wait ]; then
            if [ -f "$LOCKDIR/pid" ]; then
                echo "claude-sync: lock held by PID $(cat "$LOCKDIR/pid" 2>/dev/null || echo 'unknown')" >&2
            fi
            return 1
        fi
        sleep 0.2
    done
    if ! echo $$ > "$LOCKDIR/pid" 2>/dev/null; then
        rm -rf "$LOCKDIR"
        return 1
    fi
    return 0
}

release_lock() {
    if [ ! -d "$LOCKDIR" ]; then
        return
    fi
    if [ -f "$LOCKDIR/pid" ] && [ "$(cat "$LOCKDIR/pid" 2>/dev/null)" = "$$" ]; then
        rm -rf "$LOCKDIR"
    else
        echo "claude-sync: warning: could not verify lock ownership, lock not released" >&2
    fi
}

# Session ID = grandparent PID (the claude process)
get_session_id() {
    ps -o ppid= -p $PPID 2>/dev/null | tr -d ' '
}
