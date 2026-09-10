// Package verified is the only public path that turns an
// authenticated principal into a TenantAccess or ResourceAccess proof.
//
// The discipline:
//
//   - shenguard.NewTenantAccess / NewResourceAccess are technically
//     exported by the code generator. Callers MUST NOT call them
//     directly from outside this package. The local
//     bin/shenguard-audit.sh checks this discipline by grepping the
//     source tree (see Step 2b in the script).
//
//   - The functions below are the only auditor-checked path from
//     "have an authenticated principal" to "have a proof that the
//     principal can act on a tenant / resource". Each function:
//       (1) reads the user-id directly from the principal — no
//           separately-passed string parameter that could disagree;
//       (2) runs a SQL membership / ownership check;
//       (3) calls the shenguard constructor with the boolean result.
//
// TCB:
//
//   This package is part of the TCB. An auditor should read the SQL
//   strings, the principal-unwrapping, and the constructor call in
//   each function below. The structural chain downstream of this
//   package (the handler signatures that demand verified.TenantAccess /
//   verified.ResourceAccess parameters) prevents accidental bypass.
//
// See ../../specs/core.shen for the verified premises these wrappers
// discharge: `(= IsMember true)` for tenant-access and
// `(= IsOwned true)` for resource-access. See ../../AUDIT.md for the
// auditor workflow.
package verified

import (
	"context"
	"database/sql"
	"fmt"

	"multi-tenant-api/internal/shenguard"
)

// TenantAccess is a proof that the carried principal is a member of
// the carried tenant. Re-exports shenguard.TenantAccess so handlers
// can import only the verified package and ignore shenguard at the
// type level.
type TenantAccess = shenguard.TenantAccess

// ResourceAccess is a proof that the carried tenant owns the carried
// resource. Re-exports shenguard.ResourceAccess for the same reason
// as TenantAccess.
type ResourceAccess = shenguard.ResourceAccess

// CheckTenantAccess is a thin ergonomic wrapper (post-2).
//
// It attaches the DB to the context (via shenguard.WithDB) and calls
// the generated shenguard.NewTenantAccess. The actual membership
// decision is performed by the spec-owned runtime checker
// `checkTenantMembership` in shenguard/checkers.go.
//
// TCB notes:
// - The SQL query now lives in the checker (see checkers.go).
// - The wrapper still derives the user-id directly from the
//   principal (W2.1 structural guarantee).
// - Callers should prefer this function for normal use; the
//   guard constructor itself now enforces that a runtime check
//   occurred.
func CheckTenantAccess(ctx context.Context, db *sql.DB, principal shenguard.AuthenticatedPrincipal, tenantID shenguard.TenantId) (TenantAccess, error) {
	// Post-2 evolution: attach the DB so the :runtime-via checker
	// (checkTenantMembership in shenguard/checkers.go) can perform
	// the authoritative membership query. The guard constructor now
	// owns the decision.
	ctx = shenguard.WithDB(ctx, db)

	// The 4th argument is kept only for signature compatibility with
	// the current shengen output for this :runtime-via premise.
	// The checker ignores it completely and performs the real query.
	access, err := shenguard.NewTenantAccess(ctx, principal, tenantID, shenguard.IgnoredMembershipClaim)
	if err != nil {
		userID, _ := userIDFromPrincipal(principal)
		return shenguard.TenantAccess{}, fmt.Errorf("tenant access denied: %s is not a member of tenant %s (enforced by runtime-via checker)", userID, tenantID.Val())
	}
	return access, nil
}

// CheckResourceAccess runs a SQL ownership query and asks shenguard
// to construct a ResourceAccess. The tenant dimension is structurally
// bound: `access.Tenant().Val()` cannot be substituted by the caller,
// it comes from the TenantAccess proof.
//
// TCB: the SQL query must be exact. Read it before trusting the chain.
func CheckResourceAccess(ctx context.Context, db *sql.DB, access TenantAccess, resourceID shenguard.ResourceId) (ResourceAccess, error) {
	// Resource ownership check is still direct for now (no :runtime-via
	// annotation on that premise yet). We keep the query here.
	var exists int
	err := db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM resources WHERE id = ? AND tenant_id = ?",
		resourceID.Val(), access.Tenant().Val(),
	).Scan(&exists)
	if err != nil {
		return shenguard.ResourceAccess{}, fmt.Errorf("check resource ownership: %w", err)
	}

	isOwned := exists > 0
	ra, err := shenguard.NewResourceAccess(access, resourceID, isOwned)
	if err != nil {
		return shenguard.ResourceAccess{}, fmt.Errorf("resource access denied: resource %s is not owned by tenant %s", resourceID.Val(), access.Tenant().Val())
	}
	return ra, nil
}

// userIDFromPrincipal extracts the user-id string from a human
// principal. Returns ("", false) for service principals.
//
// The user-id reaches this function through the W2.1 structural
// chain: `principal.Auth().User()` is byte-equal to `(sub Claims)`
// inside the JWT (enforced at construction by
// `(= User (head (head Jwt))) : verified`). So the string returned
// here is, by type, the JWT's `sub` claim.
func userIDFromPrincipal(principal shenguard.AuthenticatedPrincipal) (string, bool) {
	human, ok := principal.(shenguard.HumanPrincipal)
	if !ok {
		return "", false
	}
	return human.Auth().User().Val(), true
}
