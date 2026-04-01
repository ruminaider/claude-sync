#!/bin/bash
# Stub claude-sync binary for smoke tests.
# Simulates pull/push with configurable delay.
# Set STUB_DELAY (seconds) to control latency. Default: 0.1
# Set STUB_EXIT to control exit code. Default: 0
delay="${STUB_DELAY:-0.1}"
exit_code="${STUB_EXIT:-0}"
sleep "$delay"
exit "$exit_code"
