# Shen Web Tools — Auditor Workflow

> One-page reviewer workflow for the polyglot Shen-on-SBCL +
> TypeScript-frontend demo and its tag-block resolver proof gate.
> Read [`docs/TRUST-MODEL.md`](../../docs/TRUST-MODEL.md) first for
> the project-level trust model.

## What this demo proves

A research assistant where the application logic lives in Shen,
running natively on SBCL (Common Lisp), with a TypeScript/Arrow.js
frontend. Two distinct verification surfaces:

1. **Live Shen runtime.** The CL backend loads Shen at boot
   (`backend/shen-interop.lisp:23-50`) and runs `tc +` on
   `specs/core.shen` and `specs/medicare.shen` before serving
   requests. A spec that fails to type-check refuses to boot the
   server.

2. **TypeScript proof gate for the tag/ref-table resolver.**
   `specs/tag-block-resolver.shen` declares the resolver contract:
   given a tag-block and a ref-table, classify the outcome as
   `signed-complete` (every child resolves, root signed),
   `unsigned-complete` (every child resolves, root unsigned), or
   `partial` (at least one child missing). The hand-written
   resolver `runtime/tag_resolver.ts` is pinned by
   `runtime/tag_resolver.shen-derive.test.ts`, a generated
   spec-equivalence test.

Additionally, `specs/core.shen` declares the **grounded-source**
invariant: an AI summary's source page URL must match the search
hit URL it was paired with. The `GroundedSource.createOrThrow`
constructor at `runtime/guards_gen.ts:253-255` enforces this
cross-field predicate at runtime.

## Auditor steps

### 1. Verify the spec hashes

```bash
cd examples/shen-web-tools
sha256sum specs/core.shen specs/tag-block-resolver.shen specs/medicare.shen
```

Pin these hashes against the commit you are auditing. For the
Go-based examples (`payment/`, `multi-tenant-api/`) the
`spec.files[].sha256` entries inside `transcript/discharge_report.json`
provide the same anchor; this example doesn't ship a discharge
report (see step 2 below for why), so the spec hash you compute
here *is* the audit anchor for the specs themselves.

### 2. Read the rendered audit report (caveat: not committed for this example)

The Go-based examples (`payment/`, `multi-tenant-api/`) commit a
`transcript/discharge_report.json` and rendered
`transcript/audit_report.md` so a reader on GitHub can inspect the
audit artifact cold. **shen-web-tools does not** — `sb derive` on
this example runs the TS sampler via `shen-derive-ts` and writes
the generated test (`runtime/tag_resolver.shen-derive.test.ts`) but
does not yet emit a per-spec discharge report JSON. The
`shen-derive-ts` CLI under `cmd/shen-derive-ts/` is a follow-up
gap noted in the W1.2 audit-artifacts PR body.

The audit surface that *is* committed for this example:

- **`runtime/tag_resolver.shen-derive.test.ts`** — the generated
  spec-equivalence test. Read this first: it enumerates the
  sampled inputs and the spec-vs-impl assertions that pin
  `tag_resolver.ts` to `specs/tag-block-resolver.shen`. The
  three correlated fixture rows (`signed-complete`,
  `unsigned-complete`, `partial`) are prepended deterministically.
- **`runtime/guards_gen.ts`** — the generated guard types for
  `specs/core.shen` (`GroundedSource`, etc.). The
  `GroundedSource.createOrThrow` constructor at lines 253-255
  enforces the cross-field URL-match predicate.
- **`specs/{core,tag-block-resolver,medicare}.shen`** — the
  formal specs. The spec hashes (step 1) are the audit anchor.

To regenerate a discharge report once `shen-derive-ts` grows the
per-spec report emitter, the pattern will mirror the other
examples: `sb gates && cp .sb/discharge_report.json
transcript/discharge_report.json && sb audit-report >
transcript/audit_report.md`. Until then, run `npm test` and `npm
run shen-derive-check` to verify the committed artifacts are still
in sync with the specs.

### 3. Re-run the gates at the recorded commit

Pick the commit you want to audit (the latest `main` or the SHA
referenced by the PR/release you are reviewing — for this example
there is no `discharge_report.json` to read it from yet, so use
`git log` to find the relevant commit).

```bash
git checkout <commit-sha>
cd examples/shen-web-tools
npm install
../../bin/sb gates
```

Expected output:

```
PASS  shengen        ~50ms
PASS  test           ~1.2s
PASS  build          ~3s
PASS  shen-check     ~150ms     ← needs shen-sbcl
PASS  tcb-audit      ~15ms      ← Go-only today; TS drift not yet caught
PASS  shen-derive-ts ~400ms

6/6 gates passed
```

**Important caveat for this demo:** `bin/shenguard-audit.sh` is
currently Go-only — it does not catch drift in the TypeScript
generated guards under `runtime/guards_gen.ts`. The `sb.toml`
declares `lang = "ts"` and `tcb-audit` together, but `tcb-audit`
on a TS project is currently a no-op for this purpose. Wave-2 of
the HN-follow-up plan generalizes the script with a `--lang` flag;
until then, drift in the TS guards is caught by the build (`npx
tsc --noEmit`) and the runtime tests rather than by a dedicated
drift gate. **Read this audit acknowledging that caveat.**

### 4. Read the TCB for this demo

| Piece | Where | What you're checking |
|---|---|---|
| **Shen runtime startup** | `backend/shen-interop.lisp:23-50, 172-183` | `(ql:quickload :shen)` and `(tc +)` on the spec files. If shen-sbcl mis-checks the spec, every downstream guarantee is suspect. shen-sbcl is an external dependency we trust. |
| **CL bridge providers** | `backend/bridge.lisp` | Pluggable I/O (DuckDuckGo, rho-cli, Brave, Anthropic, mock). The provider you configure is in your TCB. The default DuckDuckGo + dexador combo has no API keys, so no third-party trust beyond the libraries themselves. |
| **HTTP server hot path** | `backend/server.lisp:88-131` | Plain Common Lisp. The hot path is **not** running Shen at request time — Shen runs at boot for type-checking and the application `.shen` files are loaded as compiled CL. Read this code as ordinary CL. |
| **TypeScript bridge** | `runtime/bridge.ts:52-96` | HTTP client to the CL backend. No Shen runs in the browser. |
| **`tag_resolver.ts`** | `runtime/tag_resolver.ts` | Hand-written; pinned by the generated `shen-derive` test. Read this once. The committed test is the drift gate. |
| **`grounded-source` constructor** | `runtime/guards_gen.ts:253-255` | The cross-field check `page.url() === hit.url()`. Verify the URL normalization (case, trailing slash, query-string handling) matches the search-hit and fetched-page sources. |

### 5. Watch the tag-resolver gate in action

```bash
cd examples/shen-web-tools
npm run shengen        # regenerate runtime/guards_gen.ts
npm run shen-derive    # regenerate runtime/tag_resolver.shen-derive.test.ts
npm run check          # shengen drift + tsc + runtime tests
```

The derive sampler prepends three correlated fixture rows:
signed-complete, unsigned-complete, partial. These are the
phase-critical cases — a hand-written `resolve` that returns the
wrong outcome on any of them is caught by the generated test.

The pool composition is documented in
`runtime/tag_resolver.shen-derive.test.ts` (generated, but
human-readable). Read it.

### 6. Understanding the multi-language story for this demo

This demo emits **TypeScript** guards from
`specs/tag-block-resolver.shen`. The same emitter pipeline
(`cmd/shengen-ts`) is what produces the guard types for the
frontend. The Python and Rust emitters under `cmd/shengen-py/` and
`cmd/shengen-rs/` exist but are not invoked from `sb gen` (today;
Wave-2 of the HN-follow-up plan adds them). For this demo, the
auditable surface is the TS code in `runtime/`.

## What this demo does NOT claim

- That the search-hit URL canonicalization is robust. The
  `GroundedSource` constructor checks `page.url() === hit.url()`
  with JavaScript string equality. If two URLs differ only in
  trailing slash, percent-encoding, or fragment, they are not
  equal — and the binding holds. But this also means a search
  provider that returns canonicalized URLs while the fetch path
  returns non-canonicalized ones will produce a constructor failure
  rather than a silent mismatch. That's the conservative behavior;
  it is also a brittleness the implementer should be aware of.
- That the SBCL backend's HTTP path has been audited for
  denial-of-service amplification or memory exhaustion. The Shen
  runtime is loaded once at boot; subsequent requests use
  pre-compiled CL.
- That the live Shen runtime composes with the runtime guard
  checks. Today the Shen runtime is load-time only — it does not
  evaluate predicates on each request. (Wave-3 of the HN-follow-up
  plan prototypes `:runtime-via shen-eval` to compose them.)
- That the AI summarization itself is faithful to the sources. The
  `grounded-source` invariant binds the URL of the page to the URL
  of the search hit; it does not verify that the summary text is
  supported by the page text. That is a separate (open) problem.

For the full project-level trust model, see
[`docs/TRUST-MODEL.md`](../../docs/TRUST-MODEL.md).

## Further reading

- `README.md` — example walkthrough and architecture
- `specs/core.shen` and `specs/tag-block-resolver.shen` — the
  formal specs
- `runtime/tag_resolver.shen-derive.test.ts` — the committed
  generated spec-equivalence test (this example's primary
  reviewer-facing audit artifact; see step 2 above for why a
  rendered `transcript/audit_report.md` is not committed yet)
- For comparison, `examples/payment/transcript/audit_report.md`
  and `examples/multi-tenant-api/transcript/audit_report.md` —
  the rendered audit reports for the Go-based examples
- `../../docs/TRUST-MODEL.md` — project-level trust model
- `../../thoughts/shared/research/2026-05-05-tag-resolver-finish-line.md`
  — design notes on the resolver gate
