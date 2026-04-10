.PHONY: all build test shen-check shen-derive-verify shen-derive-regen run run-relaxed demo clean

all: build test shen-check shen-derive-verify

build:
	go build -o ralph ./cmd/ralph

test:
	go test ./...

shen-check:
	@./bin/shen-check.sh

# shen-derive-verify: regenerate the spec-equivalence test for every
# (define ...) block in specs/core.shen, fail if the committed test
# file is stale, then run the tests.
#
# The shen-derive CLI is run via `go run` from its own module directory
# (../../shen-derive) because it's a separate module.
SHEN_DERIVE_DIR  := ../../shen-derive
SHEN_DERIVE_SPEC := $(CURDIR)/specs/core.shen
SHEN_DERIVE_OUT  := $(CURDIR)/internal/derived/processable_spec_test.go
SHEN_DERIVE_PKG  := ralph-shen-agent/internal/derived
SHEN_DERIVE_GUARD := ralph-shen-agent/internal/shenguard

shen-derive-verify:
	@set -e; \
	tmp=$$(mktemp); \
	(cd $(SHEN_DERIVE_DIR) && go run . verify $(SHEN_DERIVE_SPEC) \
	  --func processable \
	  --impl-pkg $(SHEN_DERIVE_PKG) \
	  --impl-func Processable \
	  --import $(SHEN_DERIVE_GUARD) \
	  --out $$tmp); \
	if ! diff -u $(SHEN_DERIVE_OUT) $$tmp >/dev/null; then \
	  echo "DRIFT: processable_spec_test.go is stale."; \
	  echo "Run 'make shen-derive-regen' to update it."; \
	  diff -u $(SHEN_DERIVE_OUT) $$tmp || true; \
	  rm -f $$tmp; \
	  exit 1; \
	fi; \
	rm -f $$tmp; \
	go test ./internal/derived/...

shen-derive-regen:
	cd $(SHEN_DERIVE_DIR) && go run . verify $(SHEN_DERIVE_SPEC) \
	  --func processable \
	  --impl-pkg $(SHEN_DERIVE_PKG) \
	  --impl-func Processable \
	  --import $(SHEN_DERIVE_GUARD) \
	  --out $(SHEN_DERIVE_OUT)

run: build
	./ralph

run-relaxed: build
	./ralph --relaxed

demo: build
	RALPH_DEMO=1 ./ralph

clean:
	rm -f ralph
	rm -f plans/backpressure.log
