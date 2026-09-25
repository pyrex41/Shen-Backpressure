# shengen-ex

Elixir emitter for Shen-Backpressure specs. Written in Go, like `sb`. It
reuses `cmd/shengen`'s datatype parser and classifier, and
`parity_test.go` checks that the two symbol tables are identical for
every spec in the repository. Access-target selection comes from
`policyspec`, the module shen-cedar and shen-rego use.

```bash
# guards + compile tracer (+ Ash policy checks)
shengen-ex --spec specs/core.shen --namespace MyApp.Shen \
  --out lib/my_app/shen/guards_gen.ex \
  --tracer-out shen/guard_tracer.ex \
  --ash-out lib/my_app/shen/policy_gen.ex --ash-targets tenant-access,resource-access

# ExUnit (+ StreamData) spec-equivalence test against shen-erl
shengen-ex derive --spec specs/core.shen --func member-of? \
  --namespace MyApp.Shen --impl-module MyApp.Authz --impl-func member_of? \
  --stream-data --out test/derived/member_of_spec_test.exs

# drift check (used by `sb audit`)
shengen-ex ... --check
```

Normally you run it through `sb gen` / `sb derive` with `lang = "elixir"`
(see `examples/phoenix-ash-tenant/sb.toml`).

| Flag | Purpose |
|---|---|
| `--namespace` | module prefix (`MyApp.Shen` → `MyApp.Shen.TenantAccess`, `MyApp.Shen.GuardTracer`, ...) |
| `--tracer-out` | write the compile tracer (load it from `mix.exs`, see its header) |
| `--ash-out`, `--ash-targets`, `--tenant-type`, `--resource-type` | Ash `SimpleCheck`/`FilterCheck` per access rule |
| `--runtime-module` | module implementing `:runtime-via` checkers (default `<NS>.Runtime`) |
| `--all-defines` | lower every `(define ...)`, not only those premises call |
| `--strict` | fail on premises outside the supported fragment (default: fail-closed branch + warning) |
| `--check` | compare instead of writing; exit 1 on drift |
| `--dry-run` | print the symbol table (same format as `shengen --dry-run`) |

Supported premise fragment: `=`, `not`, `and`, `or`, comparisons,
`+ - * /`, `element?`, `length`, `empty?`, `shen.mod`, `head`/`tail`
chains, list literals, and calls to spec defines. Define bodies also
support `if`, `let`, `cons`, `append`, `reverse`, `cn`, `nth`, the type
predicates, `[..|..]` patterns and `where` guards. Backtracking clauses
(`<-`), lambdas and higher-order calls are rejected explicitly.

Tests: `go test ./...` runs the golden files (`testdata/golden`, refresh
with `go test -run Golden -update`), classifier parity with shengen, and
an `elixirc --warnings-as-errors` compile of the feature fixture followed
by `testdata/features_smoke.exs`. That last test is skipped without
Elixir or under `-short`.

See [docs/REFERENCE.md](../../docs/REFERENCE.md#elixir-target-cmdshengen-ex)
for the design and the list of known gaps.
