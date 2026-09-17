#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
FIXTURE="$ROOT/examples/marketplace-ops-benchmark"
STAMP="$(date -u +%Y%m%dT%H%M%SZ)"
RUN_ROOT="$ROOT/.experiments/jev-ralph/$STAMP"
BIN_DIR="$RUN_ROOT/bin"
MAX_ITER="${MAX_ITER:-4}"
PI_PROVIDER="openai-codex"
PI_MODEL="gpt-5.6-luna"
PI_THINKING="low"

mkdir -p "$BIN_DIR"
go -C "$ROOT/cmd/shengen" build -o "$BIN_DIR/shengen" .
go -C "$ROOT/cmd/sb" build -o "$BIN_DIR/sb" .

(
  cd "$FIXTURE"
  shasum -a 256 specs/core.shen internal/marketguard/guards_gen.go internal/app/services_test.go sb.toml bin/shengen-drift.sh
) > "$RUN_ROOT/immutable.sha256"

run_arm() {
  local arm="$1"
  local use_jev="$2"
  local work="$RUN_ROOT/$arm"
  cp -R "$FIXTURE" "$work"
  mkdir -p "$work/.experiment"
  : > "$work/.experiment/metrics.jsonl"

  for iteration in $(seq 1 "$MAX_ITER"); do
    local gate_log="$work/.experiment/gates-$iteration.log"
    local gate_started gate_ended gate_code
    gate_started="$(date +%s)"
    set +e
    (
      cd "$work"
      SHENGEN_BIN="$BIN_DIR/shengen" "$BIN_DIR/sb" gates
    ) > "$gate_log" 2>&1
    gate_code=$?
    set -e
    gate_ended="$(date +%s)"

    if [ "$gate_code" -eq 0 ]; then
      jq -nc --arg arm "$arm" --argjson iteration "$iteration" --argjson gate_seconds "$((gate_ended-gate_started))" \
        '{event:"complete",arm:$arm,iteration:$iteration,gate_seconds:$gate_seconds}' >> "$work/.experiment/metrics.jsonl"
      break
    fi

    local assessment_file="$work/.experiment/assessment-$iteration.json"
    local projection_file="$work/.experiment/projection-$iteration.txt"
    local jev_latency=0 jev_input=0 jev_output=0
    if [ "$use_jev" = "yes" ]; then
      (
        cd "$work"
        "$BIN_DIR/sb" assess --diagnostic-file "$gate_log" --no-cache --json
      ) > "$assessment_file"
      jev_latency="$(jq -r '.latency_ms' "$assessment_file")"
      jev_input="$(jq -r '.response.usage.input_tokens' "$assessment_file")"
      jev_output="$(jq -r '.response.usage.output_tokens' "$assessment_file")"
      jq -r '
        "JEV advisory routing (not evidence):",
        "- investigate first: \(.priority.choice) (distribution confidence \(.priority.confidence))",
        "- failure category: \(.category.choice) (distribution confidence \(.category.confidence))",
        "- additional targeted investigation useful: \(.investigate.noul)",
        "- mandatory gates remain unchanged"
      ' "$assessment_file" > "$projection_file"
    else
      printf '%s\n' "No JEV routing is available. Use the deterministic failures directly." > "$projection_file"
    fi

    local prompt_file="$work/.experiment/prompt-$iteration.md"
    {
      cat "$work/PROMPT.md"
      printf '\n\n## Experiment iteration\n\nIteration %s of %s.\n\n' "$iteration" "$MAX_ITER"
      printf '## Advisory routing\n\n'
      cat "$projection_file"
      printf '\n\n## Deterministic gate failures\n\n```text\n'
      cat "$gate_log"
      printf '\n```\n'
    } > "$prompt_file"

    local pi_trace="$work/.experiment/pi-$iteration.jsonl"
    local pi_started pi_ended pi_code
    pi_started="$(date +%s)"
    set +e
    (
      cd "$work"
      pi -p --mode json --provider "$PI_PROVIDER" --model "$PI_MODEL" --thinking "$PI_THINKING" \
        --no-session --no-extensions --no-skills --no-prompt-templates --no-context-files --approve \
        "$(cat "$prompt_file")"
    ) > "$pi_trace" 2>&1
    pi_code=$?
    set -e
    pi_ended="$(date +%s)"

    local usage_file="$work/.experiment/usage-$iteration.json"
    jq -s '[.[] | select(.type == "turn_end") | .message.usage] | last // {}' "$pi_trace" > "$usage_file"

    local integrity="pass"
    if ! (cd "$work" && shasum -a 256 -c "$RUN_ROOT/immutable.sha256" >/dev/null 2>&1); then
      integrity="tampered"
      cp "$FIXTURE/specs/core.shen" "$work/specs/core.shen"
      cp "$FIXTURE/internal/marketguard/guards_gen.go" "$work/internal/marketguard/guards_gen.go"
      cp "$FIXTURE/internal/app/services_test.go" "$work/internal/app/services_test.go"
      cp "$FIXTURE/sb.toml" "$work/sb.toml"
      cp "$FIXTURE/bin/shengen-drift.sh" "$work/bin/shengen-drift.sh"
    fi

    jq -nc \
      --arg arm "$arm" --argjson iteration "$iteration" --argjson gate_seconds "$((gate_ended-gate_started))" \
      --argjson pi_seconds "$((pi_ended-pi_started))" --argjson pi_exit "$pi_code" --arg integrity "$integrity" \
      --argjson jev_latency_ms "$jev_latency" --argjson jev_input_tokens "$jev_input" --argjson jev_output_tokens "$jev_output" \
      --slurpfile usage "$usage_file" \
      '{event:"iteration",arm:$arm,iteration:$iteration,gate_seconds:$gate_seconds,pi_seconds:$pi_seconds,pi_exit:$pi_exit,integrity:$integrity,jev:{latency_ms:$jev_latency_ms,input_tokens:$jev_input_tokens,output_tokens:$jev_output_tokens},usage:$usage[0]}' \
      >> "$work/.experiment/metrics.jsonl"
  done

  set +e
  (cd "$work" && SHENGEN_BIN="$BIN_DIR/shengen" "$BIN_DIR/sb" gates) > "$work/.experiment/final-gates.log" 2>&1
  local final_code=$?
  set -e
  jq -nc --arg arm "$arm" --argjson passed "$([ "$final_code" -eq 0 ] && echo true || echo false)" \
    '{event:"final",arm:$arm,passed:$passed}' >> "$work/.experiment/metrics.jsonl"
}

# Run the control first and preserve complete traces. Re-run with ARM_ORDER=jev-first
# for a counterbalanced replication if desired.
if [ "${ARM_ORDER:-control-first}" = "jev-first" ]; then
  run_arm jev yes
  run_arm control no
else
  run_arm control no
  run_arm jev yes
fi

jq -s '
  def summary($arm):
    [.[] | select(.arm == $arm)] as $rows |
    {
      arm: $arm,
      passed: ([$rows[] | select(.event == "final") | .passed] | last),
      iterations: ([$rows[] | select(.event == "iteration")] | length),
      pi_seconds: ([$rows[] | select(.event == "iteration") | .pi_seconds] | add // 0),
      input_tokens: ([$rows[] | select(.event == "iteration") | .usage.input] | add // 0),
      cache_read_tokens: ([$rows[] | select(.event == "iteration") | .usage.cacheRead] | add // 0),
      output_tokens: ([$rows[] | select(.event == "iteration") | .usage.output] | add // 0),
      total_tokens: ([$rows[] | select(.event == "iteration") | .usage.totalTokens] | add // 0),
      cost_usd: ([$rows[] | select(.event == "iteration") | .usage.cost.total] | add // 0),
      jev_latency_ms: ([$rows[] | select(.event == "iteration") | .jev.latency_ms] | add // 0),
      jev_input_tokens: ([$rows[] | select(.event == "iteration") | .jev.input_tokens] | add // 0),
      jev_output_tokens: ([$rows[] | select(.event == "iteration") | .jev.output_tokens] | add // 0),
      integrity_violations: ([$rows[] | select(.event == "iteration" and .integrity != "pass")] | length)
    };
  [summary("control"), summary("jev")]
' "$RUN_ROOT/control/.experiment/metrics.jsonl" "$RUN_ROOT/jev/.experiment/metrics.jsonl" > "$RUN_ROOT/summary.json"

printf 'Experiment: %s\n' "$RUN_ROOT"
jq . "$RUN_ROOT/summary.json"
