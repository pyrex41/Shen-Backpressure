# phoenix-ash-tenant — Auditor Workflow

Read [`docs/TRUST-MODEL.md`](../../docs/TRUST-MODEL.md) first. This page
covers what changes for the Elixir target.

## What is enforced, and by what

| Claim | Mechanism | Strength |
|---|---|---|
| Every guard value passed its premises | `new/N` in `lib/tenant/shen/guards_gen.ex` (generated, drift-checked by `sb audit`) | runtime check at construction |
| Guards are only built by `new/N` | compile tracer (`shen/guard_tracer.ex`) refuses `%Guard{}` literals, `%Guard{g \| …}` and `struct/2` in modules that name guard types | compile time, syntactic |
| Forgeries the tracer cannot see | `@opaque t` + Dialyzer (`make dialyzer`); `sb audit` refuses raw `__struct__` maps in `lib/` | static, see gaps below |
| Ash only serves proof holders | generated `Tenant.Shen.Policy.*Check` require `is_struct(actor, Proof)` and a tenant match | runtime policy |
| `Tenant.Authz` pure functions match the spec | `test/derived/*` vs the Shen `define` run by shen-erl | sampled (boundary pool + StreamData) |
| Each premise is load-bearing | `sb mutate-spec` over `specs/hostile/` | deductive, per premise |

## Known gaps (Elixir is dynamic)

1. **Map-update syntax** (`%{proof | tenant: t}`) and `Map.put/3` produce
   a struct that passes `is_struct/2`. Compiler tracers never see map
   updates. Dialyzer reports them only when the value reaches a function
   with an opaque contract (an accessor or a `@spec` naming
   `TenantAccess.t()`). A forged value passed straight to Ash as
   `actor:` gets no warning. We checked both cases with a probe module:
   3/3 flagged with specs, 2/4 without.
2. **Dynamic module values.** `struct(mod, …)` in a module that never
   names a guard type, called with a guard module atom from elsewhere,
   passes the tracer.
3. **TCB code**: `Tenant.Authn` (HMAC, constant-time compare) and
   `Tenant.Directory` (membership/roles) decide the I/O-backed premises
   (`IsMember`, the signature). Read them.

## Reviewer steps

```bash
cd examples/phoenix-ash-tenant
make ci                    # 7 gates + Dialyzer (0 errors expected)
sha256sum specs/core.shen  # pin the spec you reviewed
cat .sb/mutation_report.json | jq '.premises[] | {datatype, premise, status, killed_by}'
make demo                  # tracer, shen-derive, mutate-spec each catch a planted bug
```
