package shenguard

import (
	"context"
	"database/sql"
	"fmt"
)

// This file contains the :runtime-via checker implementations for the
// multi-tenant-api example. It is the post-2 demonstration of "one Shen
// spec line produces both a compile-time guard and a load-bearing runtime
// decision procedure."
//
// The key example is checkTenantMembership, wired from the premise in
// specs/core.shen:
//   (= IsMember true) : verified; \* :runtime-via checkTenantMembership *\

// dbKey is the private context key for carrying the *sql.DB into
// runtime-via checkers. This lets the generated constructors (which only
// receive ctx + the proof values) reach the database without changing
// every handler signature.
type dbKey struct{}

// WithDB attaches a database handle to the context for use by
// runtime-via checkers. Call this near the top of your request handler
// or in a middleware after you open the DB.
//
// Direct callers of generated constructors that use :runtime-via
// (e.g. NewTenantAccess) must ensure the DB is attached, otherwise
// the checker will fail with a clear error.
func WithDB(ctx context.Context, db *sql.DB) context.Context {
	if db == nil {
		return ctx
	}
	return context.WithValue(ctx, dbKey{}, db)
}

// dbFromContext retrieves the DB previously attached with WithDB.
func dbFromContext(ctx context.Context) (*sql.DB, bool) {
	db, ok := ctx.Value(dbKey{}).(*sql.DB)
	return db, ok
}

// checkTenantMembership is the real implementation of the :runtime-via
// for the tenant-access membership premise.
//
// It performs the authoritative SQL lookup using the principal and
// tenant values passed by the generated constructor. The boolean that
// used to be passed in from the caller is now advisory only (the
// checker decides).
//
// This is the evolution the post-2 write-up shows:
//   - Before: verified.CheckTenantAccess did the query then called
//     NewTenantAccess(..., isMember).
//   - After: the spec owns the decision. The constructor forces the
//     check; the checker does the real work.
//
// The DB is supplied via context (WithDB) for the demo. A production
// version could use a registered provider or the :requires-db
// extension to the runtime-via grammar.
func checkTenantMembership(ctx context.Context, predicate string, args ...any) (bool, error) {
	if len(args) < 2 {
		return false, fmt.Errorf("checkTenantMembership: expected principal and tenant args")
	}

	principal, ok := args[0].(AuthenticatedPrincipal)
	if !ok {
		return false, fmt.Errorf("checkTenantMembership: bad principal arg")
	}

	tenantID, ok := args[1].(TenantId)
	if !ok {
		return false, fmt.Errorf("checkTenantMembership: bad tenant arg")
	}

	db, ok := dbFromContext(ctx)
	if !ok {
		return false, fmt.Errorf("checkTenantMembership: no DB in context (call shenguard.WithDB(ctx, db) before constructing TenantAccess)")
	}

	userID, ok := userIDFromPrincipalForChecker(principal)
	if !ok {
		return false, fmt.Errorf("checkTenantMembership: service principals not supported for tenant membership")
	}

	var exists int
	err := db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM tenant_memberships WHERE user_id = ? AND tenant_id = ?",
		userID, tenantID.Val(),
	).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("check tenant membership (user=%s, tenant=%s): %w", userID, tenantID.Val(), err)
	}

	if exists == 0 {
		return false, nil // explicit false so the generated constructor can produce a nice "rejected" error
	}
	return true, nil
}

// userIDFromPrincipalForChecker mirrors the logic in verified but lives
// here so the checker (in shenguard) can extract the user without
// importing the verified package (keeping the generated package self-contained).
func userIDFromPrincipalForChecker(principal AuthenticatedPrincipal) (string, bool) {
	human, ok := principal.(HumanPrincipal)
	if !ok {
		return "", false
	}
	return human.Auth().User().Val(), true
}

// IgnoredMembershipClaim is the value that should be passed for the
// final boolean argument to NewTenantAccess when a :runtime-via checker
// is active for the membership premise. The checker performs the real
// DB query and completely ignores this value.
//
// This constant exists purely for readability at call sites while the
// generated constructor still requires the argument (signature
// compatibility with the current shengen emitter).
const IgnoredMembershipClaim = false

