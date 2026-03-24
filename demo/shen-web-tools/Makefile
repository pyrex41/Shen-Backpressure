# Shen Web Tools — Research Assistant
#
# Architecture:
#   Backend:  Common Lisp (SBCL) + Shen
#   Frontend: Arrow.js (TypeScript, compiled to JS)
#
# Shen runs natively on SBCL. CL provides the HTTP server (hunchentoot),
# web tools (dexador), and AI API calls. TypeScript is frontend-only.

.PHONY: all frontend backend serve shengen check clean install

# --- Default: build frontend + start CL backend ---
all: frontend backend

# --- Install frontend dependencies ---
install:
	npm install

# --- Build Arrow.js frontend (TypeScript → JS) ---
frontend: install
	npx tsc

# --- Start CL/Shen backend (serves frontend too) ---
backend:
	./backend/start.sh

# --- Convenience: build + serve ---
serve: frontend backend

# --- Generate TypeScript guard types from Shen specs ---
shengen:
	npx tsx ../../cmd/shengen-ts/shengen.ts specs/core.shen --out runtime/guards_gen.ts

# --- Full verification pipeline ---
# Gate 1: shengen — regenerate guard types from Shen specs
# Gate 2: tsc    — compile frontend against regenerated types
# Gate 3: (CL compiles at load time)
# Gate 4: Shen tc+ — verify spec consistency (runs during backend boot)
check: shengen frontend
	@echo "Frontend gates passed: shengen ✓  tsc ✓"
	@echo "Backend gate (Shen tc+) runs at server startup"

# --- Clean build artifacts ---
clean:
	rm -rf dist/ node_modules/
