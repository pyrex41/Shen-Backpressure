#!/bin/bash
# Wrapper to run Shen type check and exit cleanly.
# shen-go loops on "empty stream" after EOF instead of exiting,
# so we scan output and kill the process once we have our answer.

SHEN_BIN="${1:-./bin/shen}"
SPEC_FILE="${2:-specs/core.shen}"

if [ ! -f "$SHEN_BIN" ]; then
    echo "ERROR: shen binary not found at $SHEN_BIN"
    exit 1
fi

if [ ! -f "$SPEC_FILE" ]; then
    echo "ERROR: spec file not found at $SPEC_FILE"
    exit 1
fi

# Start shen in background, reading from a pipe
TMPOUT=$(mktemp)
printf '(load "%s")\n(tc +)\n' "$SPEC_FILE" | "$SHEN_BIN" > "$TMPOUT" 2>&1 &
SHEN_PID=$!

# Wait for output, checking periodically
RESULT="unknown"
for i in $(seq 1 20); do
    sleep 0.5

    if grep -q "type error" "$TMPOUT" 2>/dev/null; then
        RESULT="type_error"
        break
    fi

    # Look for "true" anywhere (shen outputs "(1-) true")
    if grep -q "true" "$TMPOUT" 2>/dev/null; then
        RESULT="pass"
        break
    fi

    # Check if shen already died
    if ! kill -0 "$SHEN_PID" 2>/dev/null; then
        break
    fi
done

# Kill shen (it loops forever otherwise)
kill "$SHEN_PID" 2>/dev/null
wait "$SHEN_PID" 2>/dev/null

# Show relevant output (filter out empty stream noise)
grep -v "empty stream" "$TMPOUT" | head -20
rm -f "$TMPOUT"

# Report result
case "$RESULT" in
    pass)
        echo "RESULT: PASS"
        exit 0
        ;;
    type_error)
        echo "RESULT: FAIL (type error detected)"
        exit 1
        ;;
    *)
        echo "RESULT: FAIL (tc+ did not return true within 10s)"
        exit 1
        ;;
esac
