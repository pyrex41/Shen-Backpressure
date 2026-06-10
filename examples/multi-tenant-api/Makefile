.PHONY: all shengen build test shen-check audit run run-relaxed cedar-verify rego-verify shen-lua-verify policy-verify clean clean-policies

# Run all verification gates in order
all: shengen test build shen-check audit

# Gate 1: Regenerate guard types from Shen specs
shengen:
	./bin/shengen-codegen.sh specs/core.shen shenguard internal/shenguard/guards_gen.go

# Gate 2: Test
test:
	go test ./...

# Gate 3: Build
build:
	go build ./...

# Gate 4: Shen type consistency check
shen-check:
	./bin/shen-check.sh

# Gate 5: TCB audit — verify shenguard package integrity
audit:
	./bin/shenguard-audit.sh

# Gate 6 (n-way): Differential agreement (guard ctors vs Cedar vs Rego vs decidable-fragment).
# This is the soundness gate for the lowering across tiers (Cedar ⊂ Rego ⊂ Decidable-Shen-fragment).
# The cedar-verify binary (generalized) drives:
#   - shen-cedar + real `cedar authorize` (or in-proc) on emitted JSON
#   - shen-rego + `opa eval` on emitted .rego for same samples
#   - (sketch) pure-shen-fragment total-eval
# Reports + jsonl under policies/*/ ; committed samples enable drift detection.
cedar-verify:
	go run ./cmd/cedar-verify -max-samples=24

# Convenience: run the (now multi-emitter) verify focusing on Rego path.
rego-verify:
	go run ./cmd/cedar-verify -max-samples=24

# shen-lua tier: the decidable fragment evaluated on the REAL Shen kernel
# (type inhabitation via shen.typecheck — no lowering). cedar-verify picks the
# tier up automatically when luajit + shen-lua are present; this target makes
# it explicit and benches the per-check cost. Set SHEN_LUA_DIR to a shen-lua
# checkout (or `luarocks install shen`).
shen-lua-verify:
	go run ./cmd/cedar-verify -max-samples=48
	cd runtime/shen-lua && luajit policy.lua --bench

# Run Ralph loop (strict mode)
run:
	go run ./cmd/ralph

# Run Ralph loop (relaxed — test+build in parallel)
run-relaxed:
	go run ./cmd/ralph -relaxed

clean:
	rm -f bin/shengen

# Clean emitted Cedar policies + verify artifacts (sketch for the new gate)
clean-policies:
	rm -rf policies/cedar

