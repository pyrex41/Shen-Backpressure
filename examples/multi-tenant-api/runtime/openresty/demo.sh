#!/bin/sh
# demo.sh — Shen policy enforcement inside nginx, live.
#
#   SHEN_LUA_DIR=~/projects/shen/shen-lua ./demo.sh
#
# Starts OpenResty with nginx.conf (kernel boots in the nginx master, spec
# loaded as the runtime policy), fires the allow/deny matrix at it with curl,
# prints per-request policy latency from the X-Policy-Us header, and shuts
# down. Requires: openresty on PATH, shen-lua via SHEN_LUA_DIR or an
# installed `shen` rock.
set -e
cd "$(dirname "$0")"

: "${SHEN_LUA_DIR:?set SHEN_LUA_DIR to a shen-lua checkout (or luarocks install shen and set it empty)}"
export SHEN_LUA_DIR
export SB_SPEC="$PWD/../../specs/core.shen"
export SB_CONF_DIR="$PWD"
export SHEN_KERNEL_CACHE="$PWD/.shen-kernel-cache.openresty.bin"

mkdir -p logs
openresty -p "$PWD" -c nginx.conf -e logs/error.log &
NGINX_PID=$!
trap 'kill "$NGINX_PID" 2>/dev/null || true' EXIT INT TERM

# wait for the kernel to boot inside nginx
for i in $(seq 1 50); do
  if curl -sf -o /dev/null http://127.0.0.1:8127/healthz 2>/dev/null; then break; fi
  sleep 0.2
done
curl -sf -o /dev/null http://127.0.0.1:8127/healthz || { echo "nginx did not come up"; exit 1; }

req() {  # req <user> <tenant> <resource> <expected>
  out=$(curl -s -w '\t%{http_code}' -H "X-User: $1" "http://127.0.0.1:8127/api/$2/$3")
  body=${out%	*}; code=${out##*	}
  us=$(printf '%s' "$body" | sed -n 's/.*"policy_us":\([0-9]*\).*/\1/p')
  printf '  %-3s %s -> /api/%s/%s   HTTP %s %s\n' "$1" "${us:+(${us}us)}" "$2" "$3" "$code" "$body"
  [ "$code" = "$4" ] || { echo "  EXPECTED $4 — demo FAILED"; exit 1; }
}

echo "== the kernel's judgment, per request =="
req u1 t1 r1 200   # member of t1, t1 owns r1            -> allow
req u2 t1 r1 200   # second member                        -> allow
req u3 t1 r1 403   # u3 is not a member of t1             -> deny (membership)
req u1 t1 r2 403   # t1 does not own r2                   -> deny (ownership)
req u1 t2 r2 403   # u1 not a member of t2 (r2's owner)   -> deny (cross-tenant)
req u3 t2 r2 200   # u3 in t2, t2 owns r2                 -> allow

echo "== worker self-test (post-fork kernel state) =="
curl -s http://127.0.0.1:8127/selftest | sed 's/^/  /'

echo "== latency (20 sequential requests, after warmup) =="
curl -s -o /dev/null http://127.0.0.1:8127/bench   # warm the trace cache
total=0
for i in $(seq 1 20); do
  us=$(curl -s -H "X-User: u1" http://127.0.0.1:8127/api/t1/r1 | sed -n 's/.*"policy_us":\([0-9]*\).*/\1/p')
  total=$((total + us))
done
echo "  mean policy check: $((total / 20))us in-worker (= standalone; ~1000 kernel inferences per decision)"

echo "== demo PASSED =="
