#!/bin/bash
# Ralph loop — feeds PROMPT.md into claude, runs gates, feeds errors back.
# Usage: ./run-ralph.sh
# Ctrl+C to stop between iterations.

set -e

PROMPT_FILE="PROMPT.md"
BP_MARKER="## Backpressure Errors (from previous iteration)"

# Strip old backpressure errors from prompt, keeping just the marker
reset_prompt() {
    sed -i '' "/$BP_MARKER/,\${ /$BP_MARKER/!d; }" "$PROMPT_FILE"
}

# Append gate failures to prompt as backpressure
append_backpressure() {
    local gate_name="$1"
    local output="$2"
    printf '\n### FAIL [%s]\n```\n%s\n```\n' "$gate_name" "$output" >> "$PROMPT_FILE"
}

run_gates() {
    local failed=0

    echo "  [gate] go test ./..."
    TEST_OUT=$(go test ./... 2>&1) || { echo "  FAIL [go-test]"; append_backpressure "go-test" "$TEST_OUT"; failed=1; }
    [ $failed -eq 0 ] && echo "  PASS [go-test]"

    echo "  [gate] go build ./cmd/server"
    BUILD_OUT=$(go build ./cmd/server 2>&1) || { echo "  FAIL [go-build]"; append_backpressure "go-build" "$BUILD_OUT"; failed=1; }
    [ $failed -eq 0 ] && echo "  PASS [go-build]"

    echo "  [gate] shen typecheck"
    SHEN_OUT=$(./bin/shen-check.sh 2>&1) || { echo "  FAIL [shen-typecheck]"; append_backpressure "shen-typecheck" "$SHEN_OUT"; failed=1; }
    [ $failed -eq 0 ] && echo "  PASS [shen-typecheck]"

    return $failed
}

iter=0
while true; do
    iter=$((iter + 1))
    echo ""
    echo "=== Ralph iteration $iter ==="

    # Reset backpressure section in prompt
    reset_prompt

    # Feed prompt to claude
    echo "  [ralph] Sending prompt to claude..."
    cat "$PROMPT_FILE" | claude -p --allowedTools "Edit,Write,Read,Bash,Glob,Grep" 2>&1 || true

    # Run gates
    echo "  [ralph] Running gates..."
    if run_gates; then
        echo "  [ralph] All gates PASSED on iteration $iter"
    else
        echo "  [ralph] Gates FAILED — backpressure written to PROMPT.md"
    fi

    echo ""
    echo "  Press Enter to continue, or Ctrl+C to stop..."
    read -r
done
