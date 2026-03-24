# Shen Web Tools — Research Assistant Demo
#
# Architecture: Shen logic → TypeScript bridge → Arrow UI
#
# All application logic is in src/*.shen. TypeScript only handles I/O.

.PHONY: all install build shengen serve dev check clean

# --- Default: full build + serve ---
all: install shengen build serve

# --- Install dependencies ---
install:
	npm install

# --- Generate TypeScript guard types from Shen specs ---
shengen:
	npx tsx ../../cmd/shengen-ts/shengen.ts specs/core.shen --out runtime/guards_gen.ts

# --- Compile TypeScript ---
build:
	npx tsc

# --- Serve the app ---
serve:
	npx tsx runtime/serve.ts

# --- Dev: build + serve ---
dev: install build serve

# --- Full verification pipeline ---
# Gate 1: Regenerate guard types from Shen specs
# Gate 2: TypeScript compiles against regenerated types
# Gate 3: (no Go build for this demo)
# Gate 4: Shen spec consistency (requires Shen runtime)
check: shengen build
	@echo "Gates passed: shengen ✓  tsc ✓"

# --- Clean build artifacts ---
clean:
	rm -rf dist/ node_modules/
