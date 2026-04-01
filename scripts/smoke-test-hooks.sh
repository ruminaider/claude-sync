#!/bin/bash
# Smoke test harness for claude-sync session hooks.
# Default: sandbox mode (isolated temp env). Use --live for real config.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
HOOKS_DIR="$REPO_DIR/plugin/hooks"
STUB_BIN="$SCRIPT_DIR/stub-claude-sync.sh"

MODE="sandbox"
[ "${1:-}" = "--live" ] && MODE="live"

# --- Colors and reporting ---
RED='\033[0;31m'
GREEN='\033[0;32m'
NC='\033[0m'
PASSED=0
FAILED=0
TOTAL=0

report() {
    local name="$1" status="$2" ms="$3" detail="${4:-}"
    TOTAL=$((TOTAL + 1))
    if [ "$status" = "pass" ]; then
        PASSED=$((PASSED + 1))
        printf "  ${GREEN}PASS${NC}  %-40s (%sms)\n" "$name" "$ms"
    else
        FAILED=$((FAILED + 1))
        printf "  ${RED}FAIL${NC}  %-40s (%sms)\n" "$name" "$ms"
        if [ -n "$detail" ]; then
            echo "        $detail"
        fi
    fi
}

time_ms() {
    local ms
    ms=$(date +%s%3N 2>/dev/null)
    if [[ "$ms" == *N ]]; then
        echo "$(date +%s)000"
    else
        echo "$ms"
    fi
}

# Run a hook inside two nested bash shells to simulate Claude's PID hierarchy.
# get_session_id() in lib.sh does: ps -o ppid= -p $PPID
# So we need: harness -> bash -c -> bash -c -> hook script
# The hook's PPID is the inner bash, whose PPID is the outer bash.
# get_session_id returns the outer bash's PPID, which is stable.
run_hook() {
    local hook_script="$1"
    shift
    bash -c "bash -c 'bash \"$hook_script\"' 2>&1" 2>&1
}

run_hook_bg() {
    local hook_script="$1"
    bash -c "bash -c 'bash \"$hook_script\"'" &
}

# --- Sandbox setup ---
SANDBOX=""
REAL_HOME="$HOME"

setup_sandbox() {
    SANDBOX=$(mktemp -d)
    export HOME="$SANDBOX"
    export CLAUDE_SYNC_DEBUG=1

    # Create fake config repo with local bare remote
    local sync_dir="$SANDBOX/.claude-sync"
    local remote_dir="$SANDBOX/remote.git"
    git init --bare "$remote_dir" >/dev/null 2>&1
    git init "$sync_dir" >/dev/null 2>&1
    git -C "$sync_dir" config user.email "test@test.com"
    git -C "$sync_dir" config user.name "Test"
    touch "$sync_dir/config.yaml"
    git -C "$sync_dir" add . >/dev/null 2>&1
    git -C "$sync_dir" commit -m "init" >/dev/null 2>&1
    git -C "$sync_dir" remote add origin "$remote_dir" >/dev/null 2>&1
    local branch
    branch=$(git -C "$sync_dir" branch --show-current)
    git -C "$sync_dir" push -u origin "$branch" >/dev/null 2>&1

    # Create fake CLAUDE.md
    mkdir -p "$SANDBOX/.claude"
    echo "# Test CLAUDE.md" > "$SANDBOX/.claude/CLAUDE.md"

    # Create sessions dir
    mkdir -p "$sync_dir/sessions"

    # Make stub discoverable: create a wrapper that lib.sh's resolve_claude_sync can find.
    # CLAUDE_SYNC_BIN takes priority over the bundled binary in plugin/bin/.
    mkdir -p "$SANDBOX/.local/bin"
    cp "$STUB_BIN" "$SANDBOX/.local/bin/claude-sync"
    chmod +x "$SANDBOX/.local/bin/claude-sync"
    export PATH="$SANDBOX/.local/bin:$PATH"
    export CLAUDE_SYNC_BIN="$SANDBOX/.local/bin/claude-sync"

    # Fix session ID so all hook invocations share the same session dir.
    # Each run_hook call spawns fresh bash processes, so PPID-based IDs differ.
    # A fixed ID makes session-start and stop-change-check use the same dir.
    export CLAUDE_SYNC_SESSION_ID="smoke-test-$$"
}

cleanup_sandbox() {
    if [ -n "$SANDBOX" ] && [ -d "$SANDBOX" ]; then
        export HOME="$REAL_HOME"
        rm -rf "$SANDBOX"
    fi
}

setup_live() {
    if [ ! -d "$HOME/.claude-sync" ]; then
        echo "ERROR: ~/.claude-sync does not exist. Run 'claude-sync init' first." >&2
        exit 1
    fi
    if ! command -v claude-sync >/dev/null 2>&1; then
        echo "ERROR: claude-sync not found in PATH." >&2
        exit 1
    fi
    export CLAUDE_SYNC_DEBUG=1
    # Fix session ID so all hook invocations share the same session dir.
    # run_hook spawns fresh bash processes, so PPID-based IDs differ per call.
    export CLAUDE_SYNC_SESSION_ID="smoke-test-$$"
}

# --- Test cases ---

test_session_start_happy() {
    local start end
    start=$(time_ms)
    local output
    output=$(run_hook "$HOOKS_DIR/session-start.sh" 2>&1) || true
    end=$(time_ms)
    local ms=$((end - start))

    # Verify session dir was created (check for any dir in sessions/)
    local session_count
    session_count=$(find "$HOME/.claude-sync/sessions" -mindepth 1 -maxdepth 1 -type d 2>/dev/null | wc -l | tr -d ' ')
    if [ "$session_count" -ge 1 ]; then
        report "session-start happy path" "pass" "$ms"
    else
        report "session-start happy path" "fail" "$ms" "no session dir created"
    fi
}

test_session_end_happy() {
    # First run session-start to create session state
    run_hook "$HOOKS_DIR/session-start.sh" >/dev/null 2>&1 || true

    local start end
    start=$(time_ms)
    run_hook "$HOOKS_DIR/session-end.sh" >/dev/null 2>&1 || true
    end=$(time_ms)
    local ms=$((end - start))

    # Verify our session dir was cleaned up (other active sessions may still have dirs)
    local our_session_dir="$HOME/.claude-sync/sessions/$CLAUDE_SYNC_SESSION_ID"
    if [ ! -d "$our_session_dir" ]; then
        report "session-end happy path" "pass" "$ms"
    else
        report "session-end happy path" "fail" "$ms" "session dir $CLAUDE_SYNC_SESSION_ID not cleaned up"
    fi
}

test_lock_contention() {
    local start end
    start=$(time_ms)

    # Launch two session-start hooks in parallel
    run_hook_bg "$HOOKS_DIR/session-start.sh" >/dev/null 2>&1
    local pid1=$!
    run_hook_bg "$HOOKS_DIR/session-start.sh" >/dev/null 2>&1
    local pid2=$!

    local ok=true
    wait "$pid1" 2>/dev/null || ok=false
    wait "$pid2" 2>/dev/null || ok=false
    end=$(time_ms)
    local ms=$((end - start))

    # Clean up lock if left behind
    rm -rf "$HOME/.claude-sync/.lock"

    if [ "$ok" = true ]; then
        report "lock contention resolved" "pass" "$ms"
    else
        report "lock contention resolved" "fail" "$ms" "one or both hooks failed"
    fi
}

test_dead_pid_lock() {
    local lockdir="$HOME/.claude-sync/.lock"
    mkdir -p "$lockdir"
    # Use a just-exited PID that is guaranteed dead
    bash -c 'exit 0' &
    local dead_pid=$!
    wait "$dead_pid" 2>/dev/null
    echo "$dead_pid" > "$lockdir/pid"

    local start end
    start=$(time_ms)
    run_hook "$HOOKS_DIR/session-start.sh" >/dev/null 2>&1 || true
    end=$(time_ms)
    local ms=$((end - start))

    # Clean up
    rm -rf "$lockdir"

    if [ "$ms" -lt 2000 ]; then
        report "dead PID lock broken" "pass" "$ms"
    else
        report "dead PID lock broken" "fail" "$ms" "took ${ms}ms, expected <2000ms"
    fi
}

test_timeout_behavior() {
    # Swap in a slow stub that will trigger the 5s timeout
    local real_stub="$HOME/.local/bin/claude-sync"
    cat > "$real_stub" <<'SLOWEOF'
#!/bin/bash
sleep 10
SLOWEOF
    chmod +x "$real_stub"

    local start end
    start=$(time_ms)
    local output
    output=$(run_hook "$HOOKS_DIR/session-start.sh" 2>&1) || true
    end=$(time_ms)
    local ms=$((end - start))

    # Restore normal stub
    cp "$STUB_BIN" "$real_stub"
    chmod +x "$real_stub"

    # Should complete in <7s (5s timeout + margin) and stderr should mention timeout
    if [ "$ms" -lt 7000 ] && echo "$output" | grep -qi "timed out"; then
        report "timeout returns 124" "pass" "$ms"
    else
        local detail=""
        [ "$ms" -ge 7000 ] && detail="took ${ms}ms, expected <7000ms. "
        echo "$output" | grep -qi "timed out" || detail="${detail}no timeout message in output"
        report "timeout returns 124" "fail" "$ms" "$detail"
    fi
}

test_stop_change_check() {
    # Run session-start to set up baseline hash
    run_hook "$HOOKS_DIR/session-start.sh" >/dev/null 2>&1 || true

    # Modify CLAUDE.md to trigger change detection
    echo "# Modified" >> "$HOME/.claude/CLAUDE.md"

    local start end
    start=$(time_ms)
    local output exit_code=0
    output=$(run_hook "$HOOKS_DIR/stop-change-check.sh" 2>&1) || exit_code=$?
    end=$(time_ms)
    local ms=$((end - start))

    if [ "$exit_code" -eq 2 ] && echo "$output" | grep -q '"decision":"block"'; then
        report "stop-change-check blocks on diff" "pass" "$ms"
    else
        local detail=""
        [ "$exit_code" -ne 2 ] && detail="exit code $exit_code (expected 2). "
        echo "$output" | grep -q '"decision":"block"' || detail="${detail}no decision:block in output"
        report "stop-change-check blocks on diff" "fail" "$ms" "$detail"
    fi
}

# --- Main ---

echo "smoke-hooks: $MODE mode"
echo ""

if [ "$MODE" = "sandbox" ]; then
    setup_sandbox
    trap cleanup_sandbox EXIT
else
    setup_live
fi

test_session_start_happy
test_session_end_happy
test_lock_contention
test_dead_pid_lock

# Only run timeout test in sandbox (it takes 5+ seconds and needs stub control)
if [ "$MODE" = "sandbox" ]; then
    test_timeout_behavior
fi

test_stop_change_check

echo ""
echo "$PASSED/$TOTAL passed"
if [ "$FAILED" -gt 0 ]; then
    exit 1
fi
