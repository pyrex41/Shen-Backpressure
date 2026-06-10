# Shen policy enforcement inside nginx (OpenResty)

The endgame of the decidable tier: `specs/core.shen` — the same sequent spec
the auditor read, that shengen lowered into Go guards, that Cedar/Rego were
emitted from — running **as the runtime policy inside the proxy**. No sidecar,
no policy-agent hop, no lowering: the Shen kernel boots in the nginx master
(~30 ms warm), workers fork with it, and every request to
`/api/<tenant>/<resource>` is authorized by the kernel's typechecker deciding
whether the request's ground term inhabits `resource-access`.

```
            ┌──────────────────────── nginx worker ───────────────────────┐
request ───▶│ access_by_lua:                                              │
            │   facts:    isMember ← members[tenant][user]   (data plane) │
            │   judgment: [[principal tenant isMember] resource isOwned]  │
            │             : resource-access ?          (Shen typechecker) │
            │   allow → upstream        deny → 403                        │
            └─────────────────────────────────────────────────────────────┘
```

The division of labor is the honest one: the **data plane** supplies facts
(membership, ownership — in production, from your store; here, a fixture
table), and the **kernel proves the judgment** — the full proof chain, W2.1
sub-binding included, per request.

## Run it

```sh
brew install openresty/brew/openresty --without-geoip   # macOS
SHEN_LUA_DIR=~/projects/shen/shen-lua ./demo.sh
```

The demo starts nginx, fires the allow/deny matrix (member, non-member,
wrong tenant, unowned resource, cross-tenant), runs the post-fork worker
self-test, and reports measured per-request policy latency.

## Endpoints

| | |
|---|---|
| `GET /api/<tenant>/<resource>` + `X-User: <id>` | the guarded resource: 200 with `policy_us` when the kernel proves access, 403 otherwise, 401 without identity |
| `GET /selftest` | runs the boot self-test matrix in the worker (post-fork kernel state) |
| `GET /bench` | tight in-worker loop, µs/check |
| `GET /healthz` | liveness |

## Numbers (Apple Silicon, OpenResty 1.31.1.1)

- kernel boot in the master: ~30 ms warm (shen-lua bytecode cache), ~1.5 s
  first-ever run
- policy check: **~1.0–1.1 ms** (full proof chain, ~1000 kernel inferences
  per decision) — and in-worker equals standalone (`../shen-lua/policy.lua
  --bench`): nginx adds essentially nothing. Comfortably inside a normal
  authz budget; an OPA sidecar costs a network hop before its own
  evaluation starts.

## Two operational notes (learned the hard way)

1. **Per-decision inference budget.** The kernel's inference counter
   (`shen.*infs*`) is global and cumulative; `sb_policy.allow` resets it per
   check (the kernel REPL does the same per evaluation). Without the reset, a
   long-lived worker crosses `*maxinferences*` after a few thousand checks
   and the engine fails closed on every check thereafter — caught live while
   building this demo. With the reset, `*maxinferences*` becomes a
   per-decision inference budget: a runaway check is denied, which is the
   right failure mode for a decidable tier.
2. **Fail closed everywhere.** Malformed requests, missing identity, premise
   evaluation errors, and budget exhaustion all answer deny/4xx.
