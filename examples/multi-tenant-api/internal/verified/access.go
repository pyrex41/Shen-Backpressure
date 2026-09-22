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
	"database/sql"
	"fmt"

	"multi-tenant-api/internal/shenguard"
)

// TenantAccess is a proof that the carried principal is a member of
// the carried tenant. Re-exports shenguard.TenantAccess so handlers
// can import only the verified package and ignore shenguard at the
// type level.
//
// B is the GDP brand of the proof chain (W1): the principal, this
// tenant proof and any ResourceAccess derived from it all carry the
// same brand, which is what makes a proof from another chain a compile
// error rather than a silent cross-tenant read.
type TenantAccess[B shenguard.Brand] = shenguard.TenantAccess[B]

// ResourceAccess is a proof that the carried tenant owns the carried
// resource. Re-exports shenguard.ResourceAccess for the same reason
// as TenantAccess, at the same brand.
type ResourceAccess[B shenguard.Brand] = shenguard.ResourceAccess[B]

// CheckTenantAccess derives the user-id from the principal (the W2.1
// fix), runs a SQL membership query, and asks shenguard to construct
// a TenantAccess. Returns an error if the principal is not a human
// (services use a different membership path) or if the membership
// row is missing.
//
// The brand parameter B must be named explicitly at the call site
// (`CheckTenantAccess[apibrand.API](...)`): Go does not infer type
// arguments from an interface-typed argument, and
// `authenticated-principal` is a Shen sum type, which lowers to a
// generic interface.
//
// TCB: this function's correctness rests on (a) the SQL query
// being exact, (b) `principal.Auth().User().Val()` returning the
// user-id the spec calls `(sub Claims)` — which is structurally true
// by the W2.1 cross-field premise inside `authenticated-user`. Read
// both before trusting the chain.
//
// Compared to the pre-W2.1 version, this function NO LONGER takes a
// `userID string` parameter. The pre-W2.1 signature was
// `CheckTenantAccess(db, principal, userID string, tenantID)` and the
// SQL query used the string parameter, not anything threaded from the
// principal. That gap meant the type system did not enforce that the
// queried user-id matched the authenticated user-id. With the string
// parameter dropped and the user-id read directly from the principal,
// the type system now enforces that the SQL query is keyed by the
// authenticated user.
func CheckTenantAccess[B shenguard.Brand](db *sql.DB, principal shenguard.AuthenticatedPrincipal[B], tenantID shenguard.TenantId) (TenantAccess[B], error) {
	userID, ok := userIDFromPrincipal[B](principal)
	if !ok {
		return shenguard.TenantAccess[B]{}, fmt.Errorf("service principals not supported by CheckTenantAccess")
	}

	var exists int
	err := db.QueryRow(
		"SELECT COUNT(*) FROM tenant_memberships WHERE user_id = ? AND tenant_id = ?",
		userID, tenantID.Val(),
	).Scan(&exists)
	if err != nil {
		return shenguard.TenantAccess[B]{}, fmt.Errorf("check tenant membership: %w", err)
	}

	isMember := exists > 0
	access, err := shenguard.NewTenantAccess[B](principal, tenantID, isMember)
	if err != nil {
		return shenguard.TenantAccess[B]{}, fmt.Errorf("tenant access denied: %s is not a member of tenant %s", userID, tenantID.Val())
	}
	return access, nil
}

// CheckResourceAccess runs a SQL ownership query and asks shenguard
// to construct a ResourceAccess. The tenant dimension is structurally
// bound: `access.Tenant().Val()` cannot be substituted by the caller,
// it comes from the TenantAccess proof.
//
// TCB: the SQL query must be exact. Read it before trusting the chain.
func CheckResourceAccess[B shenguard.Brand](db *sql.DB, access TenantAccess[B], resourceID shenguard.ResourceId) (ResourceAccess[B], error) {
	var exists int
	err := db.QueryRow(
		"SELECT COUNT(*) FROM resources WHERE id = ? AND tenant_id = ?",
		resourceID.Val(), access.Tenant().Val(),
	).Scan(&exists)
	if err != nil {
		return shenguard.ResourceAccess[B]{}, fmt.Errorf("check resource ownership: %w", err)
	}

	isOwned := exists > 0
	ra, err := shenguard.NewResourceAccess(access, resourceID, isOwned)
	if err != nil {
		return shenguard.ResourceAccess[B]{}, fmt.Errorf("resource access denied: resource %s is not owned by tenant %s", resourceID.Val(), access.Tenant().Val())
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
func userIDFromPrincipal[B shenguard.Brand](principal shenguard.AuthenticatedPrincipal[B]) (string, bool) {
	human, ok := principal.(shenguard.HumanPrincipal[B])
	if !ok {
		return "", false
	}
	return human.Auth().User().Val(), true
}
