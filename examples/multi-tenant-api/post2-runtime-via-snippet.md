# Post-2 Runtime-via Artifact (multi-tenant-api)

**Spec change** (specs/core.shen):
```shen
(datatype tenant-access
  ...
  (= IsMember true) : verified; \* :runtime-via checkTenantMembership *\
  ================================
  [Principal Tenant IsMember] : tenant-access;)
```

**Generated constructor** (internal/shenguard/guards_gen.go, after shengen run):
```go
func NewTenantAccess(ctx context.Context, principal AuthenticatedPrincipal, tenant TenantId, isMember bool) (TenantAccess, error) {
	ok, err := checkTenantMembership(ctx, "tenant-access", principal, tenant, isMember)
	if err != nil {
		return TenantAccess{}, fmt.Errorf("checkTenantMembership rejected tenant-access: %w", err)
	}
	if !ok {
		return TenantAccess{}, fmt.Errorf("checkTenantMembership rejected tenant-access: %w", errRuntimeCheckRejected)
	}
	return TenantAccess{...}, nil
}
```

**Compile-time witness** (same file):
```go
type runtimeChecker func(ctx context.Context, predicate string, args ...any) (bool, error)
var _ runtimeChecker = checkTenantMembership
```

**Real implementation** (internal/shenguard/checkers.go):
- Uses `WithDB(ctx, db)` to reach the database.
- Performs the authoritative `SELECT COUNT(*) FROM tenant_memberships ...` query.
- Returns the real membership result (the `isMember` argument from the old path is now advisory).

This demonstrates one Shen rule producing:
- Compile-time structural refusal (cannot construct `TenantAccess` without going through the checker).
- Runtime decision procedure (the checker does the live DB work).

See also:
- `internal/verified/access.go` (thin wrapper, updated godoc)
- `internal/verified/access_test.go` (new TestRuntimeViaCheckerIsAuthoritative showing the checker as the authoritative source)
- `AUDIT.md` (detailed reviewer guidance)
- `README.md` (Post-2 section)
- `checkers.go` (the real implementation + WithDB helper)

Status: Solid working demo.
- Build + all tests green.
- New explicit test proves the checker (not the wrapper) now owns the membership decision.
- Next natural steps: annotate more premises (JWT side), improve DB wiring patterns, explore :requires-db grammar extension.

This is one leg of the post-2 runtime backpressure spectrum.
Cedar policy generation (Way 2) scaffolding lives at `examples/cedar-policy-generation/`.