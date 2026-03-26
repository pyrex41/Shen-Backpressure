#!/usr/bin/env bash
# Start the Shen Web Tools CL backend
#
# Usage:
#   ./backend/start.sh                            # auto-detect providers
#   ANTHROPIC_API_KEY=sk-... ./backend/start.sh    # real AI + DuckDuckGo search
#   ./backend/start.sh --port 8080                 # custom port
#   ./backend/start.sh --search rho --fetch rho    # use rho-cli for web tools
#   ./backend/start.sh --search duckduckgo         # DuckDuckGo search (no API key)
#
# Provider options:
#   --search  mock|duckduckgo|rho|live   (default: auto-detect)
#   --fetch   mock|duckduckgo|rho|live   (default: auto-detect)
#   --ai      mock|anthropic|rho         (default: auto-detect from env)

set -euo pipefail
cd "$(dirname "$0")/.."

# Check SBCL
if ! command -v sbcl &>/dev/null; then
  echo "ERROR: SBCL not found."
  echo ""
  echo "Install SBCL:"
  echo "  macOS:  brew install sbcl"
  echo "  Ubuntu: sudo apt install sbcl"
  echo "  Arch:   sudo pacman -S sbcl"
  echo ""
  echo "Then install Quicklisp:"
  echo "  curl -O https://beta.quicklisp.org/quicklisp.lisp"
  echo "  sbcl --load quicklisp.lisp --eval '(quicklisp-quickstart:install)' --eval '(ql:add-to-init-file)' --quit"
  echo ""
  echo "Optional — install rho-cli for web search/fetch (Rust):"
  echo "  git clone https://github.com/pyrex41/rho.git && cd rho && cargo install --path ."
  exit 1
fi

echo "Starting Shen Web Tools on CL/SBCL..."

# Check for rho-cli
if command -v rho-cli &>/dev/null; then
  echo "  rho-cli: found ($(which rho-cli))"
else
  echo "  rho-cli: not found (DuckDuckGo search built-in, rho provider unavailable)"
fi

# Parse args into an array (avoids eval + shell injection)
SBCL_ARGS=()
while [[ $# -gt 0 ]]; do
  case "$1" in
    --port)
      SBCL_ARGS+=(--eval "(shen-web-tools::configure :port $2)")
      shift 2
      ;;
    --search)
      SBCL_ARGS+=(--eval "(shen-web-tools::configure :search :$2)")
      shift 2
      ;;
    --fetch)
      SBCL_ARGS+=(--eval "(shen-web-tools::configure :fetch :$2)")
      shift 2
      ;;
    --ai)
      SBCL_ARGS+=(--eval "(shen-web-tools::configure :ai :$2)")
      shift 2
      ;;
    --api-key)
      SBCL_ARGS+=(--eval "(shen-web-tools::configure :api-key \"$2\")")
      shift 2
      ;;
    --model)
      SBCL_ARGS+=(--eval "(shen-web-tools::configure :model \"$2\")")
      shift 2
      ;;
    --rho-bin)
      SBCL_ARGS+=(--eval "(shen-web-tools::configure :rho-bin \"$2\")")
      shift 2
      ;;
    *)
      echo "Unknown option: $1"
      echo "Usage: $0 [--port N] [--search mock|duckduckgo|rho|live] [--fetch mock|duckduckgo|rho|live] [--ai mock|anthropic|rho] [--api-key KEY]"
      exit 1
      ;;
  esac
done

# Build TypeScript frontend first (if node available)
if command -v npx &>/dev/null; then
  echo "Building Arrow.js frontend..."
  npm install --silent 2>/dev/null || true
  npx tsc 2>/dev/null || echo "  (TypeScript build skipped)"
fi

# Launch SBCL: load system, apply config, then boot
# SHEN_WEB_TOOLS_NO_AUTOBOOT prevents boot during --load so configure runs first
export SHEN_WEB_TOOLS_NO_AUTOBOOT=1
exec sbcl --dynamic-space-size 2048 \
     --load backend/load.lisp \
     "${SBCL_ARGS[@]}" \
     --eval "(shen-web-tools::boot)"
