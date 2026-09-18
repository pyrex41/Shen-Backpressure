#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
FIXTURE="$ROOT/examples/marketplace-ops-benchmark"
SEEDS="$ROOT/experiments/jev-ralph/seeds"
STAMP="$(date -u +%Y%m%dT%H%M%SZ)"
RUN_ROOT="$ROOT/.experiments/jev-ralph-multiseed/$STAMP"
BIN="$RUN_ROOT/bin"
mkdir -p "$BIN"
go -C "$ROOT/cmd/shengen" build -o "$BIN/shengen" .
go -C "$ROOT/cmd/sb" build -o "$BIN/sb" .

focus_gate() {
  local log="$1" gate="$2" out="$3"
  awk -v target="$gate" '$0=="--- FAIL [" target "] ---"{show=1;print;next} show&&/^--- FAIL \[/{exit} show{print}' "$log" > "$out"
}

failure_count() { awk '/^--- FAIL: TestContract/{n++} END{print n+0}' "$1"; }
gate_count() { local tmp; tmp="$(mktemp)"; focus_gate "$1" "$2" "$tmp"; failure_count "$tmp"; rm -f "$tmp"; }

deterministic_choice() {
  local log="$1" best="" best_n=-1 gate n
  for gate in tenant-contracts commerce-contracts workflow-contracts; do
    n="$(gate_count "$log" "$gate")"
    if [ "$n" -gt "$best_n" ]; then best="$gate"; best_n="$n"; fi
  done
  printf '%s' "$best"
}

prepare_seed() {
  local seed="$1" dst="$2"
  cp -R "$FIXTURE" "$dst"
  case "$seed" in
    tenant-workflow) cp "$SEEDS/fixed-commerce.go" "$dst/internal/app/commerce.go" ;;
    commerce-workflow) cp "$SEEDS/fixed-tenant.go" "$dst/internal/app/tenant.go" ;;
  esac
  gofmt -w "$dst/internal/app"/*.go
}

run_arm() {
  local seed="$1" arm="$2" work="$RUN_ROOT/$seed-$arm"
  prepare_seed "$seed" "$work"
  mkdir -p "$work/.experiment"
  : > "$work/.experiment/metrics.jsonl"

  for iteration in 1 2; do
    local gates="$work/.experiment/gates-$iteration.log" code before selected baseline fallback=false assessment=""
    set +e; (cd "$work" && SHENGEN_BIN="$BIN/shengen" "$BIN/sb" gates) > "$gates" 2>&1; code=$?; set -e
    before="$(failure_count "$gates")"
    if [ "$code" -eq 0 ]; then break; fi
    baseline="$(deterministic_choice "$gates")"; selected="$baseline"
    if [ "$arm" = "jev" ]; then
      assessment="$work/.experiment/assessment-$iteration.json"
      (cd "$work" && "$BIN/sb" assess --diagnostic-file "$gates" --no-cache --json) > "$assessment"
      local choice; choice="$(jq -r '.priority.choice' "$assessment")"; choice="${choice#gate:}"
      if [ "$(gate_count "$gates" "$choice")" -gt 0 ]; then selected="$choice"; else fallback=true; fi
    fi

    local focused="$work/.experiment/focused-$iteration.log" prompt="$work/.experiment/prompt-$iteration.md"
    focus_gate "$gates" "$selected" "$focused"
    for domain in tenant commerce workflow; do cp "$work/internal/app/$domain.go" "$work/.experiment/before-$iteration-$domain.go"; done
    cp "$work/internal/app/services.go" "$work/.experiment/before-$iteration-services.go"
    cp "$work/internal/app/services_test.go" "$work/.experiment/before-$iteration-services_test.go"
    cp "$work/specs/core.shen" "$work/.experiment/before-$iteration-core.shen"
    cp "$work/internal/marketguard/guards_gen.go" "$work/.experiment/before-$iteration-guards_gen.go"
    cp "$work/sb.toml" "$work/.experiment/before-$iteration-sb.toml"
    {
      cat "$work/PROMPT.md"
      printf '\n\nRepair only `%s` in this turn. Only `internal/app/%s.go` may be edited; changes elsewhere will be restored.\n\n```text\n' "$selected" "${selected%-contracts}"
      cat "$focused"; printf '\n```\n'
    } > "$prompt"
    local trace="$work/.experiment/pi-$iteration.jsonl" started ended
    started="$(date +%s)"
    (cd "$work" && pi -p --mode json --provider openai-codex --model gpt-5.6-luna --thinking low --no-session --no-extensions --no-skills --no-prompt-templates --no-context-files --approve "$(cat "$prompt")") > "$trace" 2>&1
    ended="$(date +%s)"

    local rejected=0 selected_domain="${selected%-contracts}"
    for domain in tenant commerce workflow; do
      if [ "$domain" != "$selected_domain" ] && ! cmp -s "$work/internal/app/$domain.go" "$work/.experiment/before-$iteration-$domain.go"; then
        cp "$work/.experiment/before-$iteration-$domain.go" "$work/internal/app/$domain.go"; rejected=$((rejected+1))
      fi
    done
    for protected in services.go services_test.go; do
      if ! cmp -s "$work/internal/app/$protected" "$work/.experiment/before-$iteration-$protected"; then cp "$work/.experiment/before-$iteration-$protected" "$work/internal/app/$protected"; rejected=$((rejected+1)); fi
    done
    if ! cmp -s "$work/specs/core.shen" "$work/.experiment/before-$iteration-core.shen"; then cp "$work/.experiment/before-$iteration-core.shen" "$work/specs/core.shen"; rejected=$((rejected+1)); fi
    if ! cmp -s "$work/internal/marketguard/guards_gen.go" "$work/.experiment/before-$iteration-guards_gen.go"; then cp "$work/.experiment/before-$iteration-guards_gen.go" "$work/internal/marketguard/guards_gen.go"; rejected=$((rejected+1)); fi
    if ! cmp -s "$work/sb.toml" "$work/.experiment/before-$iteration-sb.toml"; then cp "$work/.experiment/before-$iteration-sb.toml" "$work/sb.toml"; rejected=$((rejected+1)); fi
    local usage="$work/.experiment/usage-$iteration.json"
    jq -s '[.[]|select(.type=="turn_end")|.message.usage]|last//{}' "$trace" > "$usage"
    jq -nc --arg seed "$seed" --arg arm "$arm" --arg selected "$selected" --arg baseline "$baseline" --argjson iteration "$iteration" --argjson failures_before "$before" --argjson seconds "$((ended-started))" --argjson fallback "$fallback" --argjson rejected "$rejected" --slurpfile usage "$usage" '{seed:$seed,arm:$arm,iteration:$iteration,selected:$selected,baseline:$baseline,failures_before:$failures_before,seconds:$seconds,fallback:$fallback,rejected_cross_domain_edits:$rejected,usage:$usage[0]}' >> "$work/.experiment/metrics.jsonl"
  done

  local final="$work/.experiment/final-gates.log" final_code final_failures
  set +e; (cd "$work" && SHENGEN_BIN="$BIN/shengen" "$BIN/sb" gates) > "$final" 2>&1; final_code=$?; set -e
  final_failures="$(failure_count "$final")"
  jq -nc --arg seed "$seed" --arg arm "$arm" --argjson passed "$([ "$final_code" -eq 0 ]&&echo true||echo false)" --argjson final_failures "$final_failures" '{seed:$seed,arm:$arm,event:"final",passed:$passed,final_failures:$final_failures}' >> "$work/.experiment/metrics.jsonl"
}

for seed in all-domains tenant-workflow commerce-workflow; do
  if [ "$seed" = tenant-workflow ]; then run_arm "$seed" jev; run_arm "$seed" deterministic; else run_arm "$seed" deterministic; run_arm "$seed" jev; fi
done

find "$RUN_ROOT" -path '*/.experiment/metrics.jsonl' -print0 | xargs -0 jq -s '.' > "$RUN_ROOT/all-metrics.json"
printf 'Experiment: %s\n' "$RUN_ROOT"
jq '[.[]|select(.iteration==1)|{seed,arm,selected,baseline,failures_before,rejected_cross_domain_edits}]' "$RUN_ROOT/all-metrics.json"
jq '[.[]|select(.event=="final")|{seed,arm,passed,final_failures}]' "$RUN_ROOT/all-metrics.json"
