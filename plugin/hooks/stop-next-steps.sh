#!/bin/bash
# Stop hook: block agent from stopping without communicating next steps
#
# Logic:
#   1. If no transcript, allow stop
#   2. If this session has no task list, allow stop (Q&A, no tracked work)
#   3. If a task list exists (any status), check transcript tail for next-steps language
#   4. If no next-steps language found, block with re-grounding prompt

set -euo pipefail

input=$(cat)
transcript_path=$(echo "$input" | jq -r '.transcript_path // empty')
session_id=$(echo "$input" | jq -r '.session_id // empty')

# No transcript -> allow stop
if [ -z "$transcript_path" ] || [ ! -f "$transcript_path" ]; then
    exit 0
fi

# Scope to current session's task list only
TASKS_DIR="$HOME/.claude/tasks"
SESSION_TASKS="$TASKS_DIR/$session_id"
has_tasks=false

if [ -n "$session_id" ] && [ -d "$SESSION_TASKS" ]; then
    for task_file in "$SESSION_TASKS"/*.json; do
        [ -f "$task_file" ] || continue
        has_tasks=true
        break
    done
fi

# No task list for this session -> allow stop (Q&A session, no tracked work)
if [ "$has_tasks" = false ]; then
    exit 0
fi

# Check transcript tail for next-steps language (last 200 lines covers final messages)
tail_content=$(tail -200 "$transcript_path" 2>/dev/null || true)

if echo "$tail_content" | grep -iqE \
    'next step|remaining (task|work|item)|still (need|left) to|what.s (next|left|remaining)|upcoming|trajectory|pending task|follow.?up.*(task|item|work)|here.s what.s left|moving forward|plan (calls for|includes)|to.do list|task list|all (tasks|work) (is |are )?(now )?complete|everything.s (done|complete)|no remaining'; then
    exit 0
fi

# Block: task list exists but agent didn't communicate trajectory
reason="This session has a task list but you haven't communicated next steps. Before stopping, briefly tell the user: (1) what was completed, (2) what remains or what the next phase is, and (3) what the recommended next action is. If everything is truly done, say so explicitly."
echo "{\"decision\":\"block\",\"reason\":\"${reason}\"}" >&2
exit 2
