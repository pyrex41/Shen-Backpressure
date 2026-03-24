#!/usr/bin/env bash
# Start the Shen Web Tools CL backend
#
# Usage:
#   ./backend/start.sh                          # mock providers
#   ANTHROPIC_API_KEY=sk-... ./backend/start.sh  # real AI
#   ./backend/start.sh --port 8080               # custom port

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
  echo "Then install Shen:"
  echo "  git clone https://github.com/Shen-Language/shen-sbcl.git"
  echo "  cd shen-sbcl && make && sudo make install"
  exit 1
fi

echo "Starting Shen Web Tools on CL/SBCL..."

# Parse args for SBCL eval expressions
EXTRA_EVAL=""
while [[ $# -gt 0 ]]; do
  case "$1" in
    --port)
      EXTRA_EVAL="$EXTRA_EVAL --eval '(shen-web-tools::configure :port $2)'"
      shift 2
      ;;
    --ai)
      EXTRA_EVAL="$EXTRA_EVAL --eval '(shen-web-tools::configure :ai :$2)'"
      shift 2
      ;;
    --api-key)
      EXTRA_EVAL="$EXTRA_EVAL --eval '(shen-web-tools::configure :api-key \"$2\")'"
      shift 2
      ;;
    *)
      echo "Unknown option: $1"
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

# Launch SBCL with the load script
eval sbcl --dynamic-space-size 2048 \
     --load backend/load.lisp \
     $EXTRA_EVAL
