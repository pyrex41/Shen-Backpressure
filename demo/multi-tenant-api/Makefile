.PHONY: all shengen build test shen-check audit run run-relaxed clean

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
	../../bin/shenguard-audit.sh

# Run Ralph loop (strict mode)
run:
	go run ./cmd/ralph

# Run Ralph loop (relaxed — test+build in parallel)
run-relaxed:
	go run ./cmd/ralph -relaxed

clean:
	rm -f bin/shengen
