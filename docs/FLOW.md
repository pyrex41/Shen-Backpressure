# Flow premises

A grep gate answers a question about spelling. A flow premise answers
a question about the program.

Both examples used to enforce "only the checked wrapper may call the
raw guard constructor" with a regex over source text. An import alias
defeats that regex without changing a line of behaviour
(`examples/multi-tenant-api/bypass_attempts/08_aliased_import.go.bak`).
A flow premise is evaluated over a *resolved symbol graph* instead, so
the alias is already gone by the time the rule runs.

This is W3 of `thoughts/shared/plans/2026-09-22-verifier-throughput-roadmap.md`.

## The forms

Flow premises live in the spec file, alongside the datatypes:

```shen
(flow tenant-access-discipline
  (constructor-only internal/shenguard/NewTenantAccess
                    internal/verified/CheckTenantAccess
                    cmd/cedar-verify/computeGuardAllow)
  (must-pass-through *ListResources*
                     internal/verified/CheckTenantAccess
                     DB#Query*))
```

### `(constructor-only <ctor> <allowed-caller>…)`

Every resolved reference to `<ctor>` must lie inside a definition
matching one of the allowed callers. Two kinds of reference are exempt
without being listed: the constructor's own definition, and any
reference from the file that defines it (a generated guard package
names its own constructors).

This is the lowering of `unsanctioned-caller/2`. It replaces the grep
gate, and it is strictly more precise in two ways: it resolves
aliases, and it can exempt a single *function* where a regex can only
exempt a whole file. The multi-tenant example exercises exactly that
difference — the Cedar differential oracle
`cmd/cedar-verify/computeGuardAllow` legitimately calls the raw
constructor, and the grep has to exempt all of
`cmd/cedar-verify/main.go` to say so.

### `(must-pass-through <source> <proof> <sink>)`

On every call path from a definition matching `<source>` to a call of
`<sink>`, some definition on the path must reference `<proof>`. With
the guard constructor as the declassifier this is the noninterference
statement: no handler reaches the database without first obtaining a
proof.

A violation reports the sink's call site and the shortest violating
path, found by a breadth-first search that prunes at any definition
referencing the proof.

**What it does not say.** The fact base records that a definition
references the proof, not that it does so *before* the sink on every
execution. Ordering inside a body is outside what a reference graph
can express. A premise that needs ordering needs a dataflow analysis,
which is a different (and much larger) tool.

### Vacuity

A premise that ranged over nothing — no reference to the constructor,
no definition matching the source — is reported **vacuous** and is
*not* discharged. A stale pattern must read as "no evidence", never as
"proved". This is the most common way a flow premise decays, since the
patterns are text and the code moves.

### Patterns

A pattern matches a symbol's canonical descriptor path: the SCIP
symbol with its four header fields (scheme, manager, package,
*version*) dropped, backticks stripped, and the trailing descriptor
punctuation removed.

```
scip-go gomod multi-tenant-api 6b9dde0 `multi-tenant-api/internal/verified`/CheckTenantAccess().
  → multi-tenant-api/internal/verified/CheckTenantAccess

scip-typescript npm shen-web-tools 0.1.0 runtime/`guards_gen.ts`/mustSignedComplete().
  → runtime/guards_gen.ts/mustSignedComplete
```

A pattern matches the whole canonical form, or any suffix of it
beginning at a `/` or `#` boundary, with `*` matching any run of
characters. So a spec never carries a module path or a version hash,
and `DB#Query` matches `database/sql/DB#Query` while leaving
`DB#QueryRow` alone (write `DB#Query*` for both).

### Why the examples put the forms in a comment

`flow` is not a Shen function, so a bare top-level `(flow …)` form
would make gate 4 (`shen tc+`) complain about an undefined symbol.
Both examples keep the forms inside a `\* … *\` block; sb's parser
reads them either way, and shengen and shen-derive ignore them in
either spelling (pinned by tests in `cmd/shengen/main_test.go` and
`shen-derive/specfile/flow_test.go`). A project whose Shen host
defines the form can write it bare.

## The pipeline

```
source tree
   |  sb index           (scip-go | scip-typescript)
   v
.sb/index.scip           SCIP index — the resolved symbol graph
   |  minimal reader     (cmd/sb/internal/scip)
   v
.sb/facts.shen           (def …) (ref …) (calls …)
   |  engine
   v
premise verdicts  ->  .sb/discharge_report.json
```

`sb index` is cached by a content hash over the source files of the
project's language, so a Ralph iteration that did not touch source
re-runs in milliseconds. `sb index --force` bypasses the cache.

The fact vocabulary is three relations and nothing else:

```
(def  sym file start-line start-col end-line end-col)
(ref  sym file line col enclosing-def-sym)
(calls caller-sym callee-sym)
```

Positions are zero-based, exactly as SCIP reports them; only the
human-facing `file:line:col` adds one. Enclosing definitions come
from range containment against the indexer's `enclosing_range`,
innermost first. Call edges are references to callable symbols (the
SCIP descriptor suffix `().`) from inside a definition — which is the
step that makes the whole thing language-independent, because the
indexer resolved receivers, imports and aliases before we saw them.

## The engines

There are two, they are compared against each other, and the report
says which ran in its `flow_engine` field and in every premise's
rationale.

**`sb/flow/stdlib.shen` — Shen's embedded Prolog. Primary.** The rules
are written once, in the same language as the spec. Facts are asserted
by *loading* `.sb/facts.shen`: `def`, `ref` and `calls` are Shen
functions that push onto three globals, so no parser ships with the
fact file. On top sit `reaches/2`, `ctor-reference`,
`unsanctioned-caller` and the `must-pass-through` violation search.
Running them needs a Shen host — any port; `make shen-go` at the
repository root builds one into `bin/shen`.

**`cmd/sb/flow` — Go.** A transcription of the same rules. It is the
engine that produces a violation's `file:line` and shortest violating
path, because Shen's `prolog?` answers satisfiability rather than
enumerating solutions. It is also the only engine when no host is
installed.

### `--engine`

```
sb flow --engine go      # the Go evaluator alone
sb flow --engine shen    # the Prolog rules alone
sb flow --engine both    # both, and fail if they disagree  (default with a host)
```

`both` asks each engine two questions per premise — did it range over
anything (the vacuity question), and is there a counterexample — and
fails the gate on any disagreement, naming the premise and each
engine's verdict. The default is `both` when a host resolves and `go`
when none does; asking for `shen` or `both` explicitly with no host is
an error rather than a silent downgrade.

This is what closes the gap this section used to record. "The two
engines implement the same rules" was a standing trust assumption that
nothing could falsify, because the Shen rules had never executed — the
file defined `(define call …)`, and `call` is a Shen system function,
so loading it failed on the first fact predicate. Running it turned up
three further bugs, each of which would have made a premise pass for
the wrong reason: a `glob?` clause missing for an exhausted pattern
(which aborted the run on any real fact base), a `drop-descriptor-tail`
that removed one character instead of the whole `().` suffix (which
made every pattern match nothing and every verdict a silent vacuous
pass), and side conditions written `(is V (f …)) (when V)`, a form that
always fails. The rename to `calls` is the reason the fact vocabulary
changed; `sb index`'s reader still accepts a `(call …)` fact so a
cached fact file from an older run still loads.

`cmd/sb/flow/stdlib_test.go` now runs the Shen engine whenever a host
resolves, with no opt-in: a load test, an end-to-end verdict test
against the fixture facts, and an agreement test between the two
engines. The `SB_FLOW_SHEN=1` opt-in is gone — an opt-in on a test
whose only job is to execute something is a way of not executing it.

The documented escape hatch for scale is a third engine: emit Soufflé
from the same rule text. Shen's Prolog over a few thousand facts is
fine; a monorepo will not be, and `sb flow` reports a host timeout
with exactly that advice.

## Degradation

With no indexer on PATH, `sb flow`:

1. warns, naming the indexer and the command that installs it;
2. runs the gate's `run` field, which for a flow gate is the *legacy
   grep* rather than the gate itself;
3. records every premise as `unproven` with basis `grep-fallback` and
   a rationale that says a regex cannot see through an aliased import;
4. fails the gate if the grep failed.

Degrading the evidence must not degrade the enforcement — but the
weaker evidence never gets to claim a discharge.

## sb.toml

```toml
[[gates]]
name = "flow"
kind = "flow"
run  = "./bin/shenguard-audit.sh --grep-only"   # fallback only
```

`kind = "flow"` is served by `sb flow`; the `run` field is the
fallback command, and may be omitted when there is no legacy grep to
fall back to. The fixed five-gate shape is untouched: `flow` is an
additional manifest gate, like `shen-derive` and the policy gates.

The host the Shen engine uses is the project's, from the same
`[shen] bin` that gate 4 reads:

```toml
[shen]
bin = "../../bin/shen"   # what `make shen-go` builds; $SHEN overrides it
```

## The TCB implication

**For flow premises, the SCIP indexer is in the TCB.** A premise
discharged with basis `flow-analysis` is trustworthy exactly as far as
the index is: a reference the indexer failed to record is a reference
the premise never saw.

The reason to accept that is the same reason the analysis works at
all: the indexer runs *after*, and on top of, the language's own type
checker. `scip-go` loads packages through `go/packages`; the Go type
checker has already resolved every import alias, embedded method and
interface satisfaction. `scip-typescript` uses the TypeScript
compiler API, likewise. So the symbol in the fact base is the symbol
the compiler resolved — which is precisely why an aliased import
cannot evade the premise, and why the indexer is not an independent
second opinion but a *reader* of the compiler's own conclusion.

Two smaller members of the same TCB:

- The minimal SCIP reader in `cmd/sb/internal/scip`. It decodes four
  fields and skips unknown ones, and treats a truncated index as a
  hard error rather than a partial decode — an understated reference
  set would make a premise look discharged when it is not.
- Whichever engine ran, per the section above.

See `docs/TRUST-MODEL.md`.

## Files

| Path | What |
|---|---|
| `cmd/sb/index.go` | `sb index`: language detection, indexer, tree-hash cache |
| `cmd/sb/internal/scip/` | minimal dependency-free SCIP reader |
| `cmd/sb/flow/` | facts, patterns, spec forms, the Go engine |
| `cmd/sb/flow_gate.go` | gate kind `flow`, discharge-report rows |
| `sb/flow/stdlib.shen` | the Shen Prolog rules (primary engine) |
| `cmd/sb/flow/testdata/tinysrc/` | fixture module; regeneration recipe in its `go.mod` |
