package verified

import (
	"database/sql"
	"testing"

	"multi-tenant-api/internal/db"
	"multi-tenant-api/internal/shenguard"
)

// makePrincipal constructs a fully-typed HumanPrincipal for the given
// user id, walking the full W2.1 proof chain:
//
//	UserId / JwtIssuer / JwtAudience
//	  → ParsedClaims (Exp > 0)
//	  → VerifiedJwt   (non-empty signature)
//	  → AuthenticatedUser (User == sub(Claims) — STRUCTURAL)
//	  → HumanPrincipal
//
// The cross-field binding is *enforced* here: if we ever passed
// `NewAuthenticatedUser(jwt, NewUserId("someone-else"))` instead of
// `NewAuthenticatedUser(jwt, NewUserId(userID))`, the constructor
// would return an error. That's the spec premise
// `(= User (head (head Jwt))) : verified` in action.
func makePrincipal(t *testing.T, userID string) (shenguard.HumanPrincipal, string) {
	t.Helper()
	iss, err := shenguard.NewJwtIssuer("multi-tenant-api")
	if err != nil {
		t.Fatalf("NewJwtIssuer: %v", err)
	}
	aud, err := shenguard.NewJwtAudience("users")
	if err != nil {
		t.Fatalf("NewJwtAudience: %v", err)
	}
	uid := shenguard.NewUserId(userID)
	claims, err := shenguard.NewParsedClaims(uid, 9999999999, iss, aud)
	if err != nil {
		t.Fatalf("NewParsedClaims: %v", err)
	}
	verifiedJwt, err := shenguard.NewVerifiedJwt(claims, "test-signature")
	if err != nil {
		t.Fatalf("NewVerifiedJwt: %v", err)
	}
	authUser, err := shenguard.NewAuthenticatedUser(verifiedJwt, uid)
	if err != nil {
		t.Fatalf("NewAuthenticatedUser: %v", err)
	}
	return shenguard.NewHumanPrincipal(authUser), userID
}

func TestCheckTenantAccessGranted(t *testing.T) {
	d, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer d.Close()
	if err := db.Seed(d); err != nil {
		t.Fatalf("Seed: %v", err)
	}

	principal, _ := makePrincipal(t, "u-alice")
	tenantID := shenguard.NewTenantId("t-acme")

	access, err := CheckTenantAccess(d, principal, tenantID)
	if err != nil {
		t.Fatalf("CheckTenantAccess: %v", err)
	}

	if access.Tenant().Val() != "t-acme" {
		t.Errorf("tenant: got %s, want t-acme", access.Tenant().Val())
	}
}

func TestCheckTenantAccessDenied(t *testing.T) {
	d, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer d.Close()
	if err := db.Seed(d); err != nil {
		t.Fatalf("Seed: %v", err)
	}

	// Bob is a member of t-globex, NOT t-acme
	principal, _ := makePrincipal(t, "u-bob")
	tenantID := shenguard.NewTenantId("t-acme")

	_, err = CheckTenantAccess(d, principal, tenantID)
	if err == nil {
		t.Fatal("expected error for non-member access, got nil")
	}
}

func TestCheckTenantAccessNonexistentUser(t *testing.T) {
	d, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer d.Close()
	if err := db.Seed(d); err != nil {
		t.Fatalf("Seed: %v", err)
	}

	principal, _ := makePrincipal(t, "u-nobody")
	tenantID := shenguard.NewTenantId("t-acme")

	_, err = CheckTenantAccess(d, principal, tenantID)
	if err == nil {
		t.Fatal("expected error for nonexistent user, got nil")
	}
}

// W2.1 hardening: the constructor refuses to build an AuthenticatedUser
// whose `User` disagrees with `(head (head Jwt))` (i.e., the sub claim
// inside the JWT). Pre-W2.1, the binding was convention-only and a
// caller could pair a JWT for Alice with a UserId for Bob. This test
// is the structural counterpart to the spec premise
// `(= User (head (head Jwt))) : verified`.
func TestCrossFieldBindingRejectsMismatch(t *testing.T) {
	iss, err := shenguard.NewJwtIssuer("multi-tenant-api")
	if err != nil {
		t.Fatalf("NewJwtIssuer: %v", err)
	}
	aud, err := shenguard.NewJwtAudience("users")
	if err != nil {
		t.Fatalf("NewJwtAudience: %v", err)
	}
	aliceID := shenguard.NewUserId("u-alice")
	bobID := shenguard.NewUserId("u-bob")

	claims, err := shenguard.NewParsedClaims(aliceID, 9999999999, iss, aud)
	if err != nil {
		t.Fatalf("NewParsedClaims: %v", err)
	}
	verifiedJwt, err := shenguard.NewVerifiedJwt(claims, "test-signature")
	if err != nil {
		t.Fatalf("NewVerifiedJwt: %v", err)
	}

	// Try to pair Alice's JWT with Bob's user id. The constructor
	// must reject this.
	_, err = shenguard.NewAuthenticatedUser(verifiedJwt, bobID)
	if err == nil {
		t.Fatal("NewAuthenticatedUser(alice-jwt, bob-id) succeeded — cross-field premise is not being enforced")
	}

	// Sanity: matching ids succeed.
	if _, err := shenguard.NewAuthenticatedUser(verifiedJwt, aliceID); err != nil {
		t.Fatalf("NewAuthenticatedUser(alice-jwt, alice-id) returned unexpected error: %v", err)
	}
}

func makeTenantAccess(t *testing.T, d *sql.DB, userID, tenantID string) shenguard.TenantAccess {
	t.Helper()
	principal, _ := makePrincipal(t, userID)
	tid := shenguard.NewTenantId(tenantID)
	access, err := CheckTenantAccess(d, principal, tid)
	if err != nil {
		t.Fatalf("CheckTenantAccess: %v", err)
	}
	return access
}

func TestCheckResourceAccessGranted(t *testing.T) {
	d, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer d.Close()
	if err := db.Seed(d); err != nil {
		t.Fatalf("Seed: %v", err)
	}

	// Alice has access to t-acme, which owns r-1
	access := makeTenantAccess(t, d, "u-alice", "t-acme")
	resourceID := shenguard.NewResourceId("r-1")

	ra, err := CheckResourceAccess(d, access, resourceID)
	if err != nil {
		t.Fatalf("CheckResourceAccess: %v", err)
	}

	if ra.Resource().Val() != "r-1" {
		t.Errorf("resource: got %s, want r-1", ra.Resource().Val())
	}
	if ra.Access().Tenant().Val() != "t-acme" {
		t.Errorf("tenant: got %s, want t-acme", ra.Access().Tenant().Val())
	}
}

func TestCheckResourceAccessDeniedCrossTenant(t *testing.T) {
	d, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer d.Close()
	if err := db.Seed(d); err != nil {
		t.Fatalf("Seed: %v", err)
	}

	// Alice has access to t-acme, but r-3 belongs to t-globex
	access := makeTenantAccess(t, d, "u-alice", "t-acme")
	resourceID := shenguard.NewResourceId("r-3")

	_, err = CheckResourceAccess(d, access, resourceID)
	if err == nil {
		t.Fatal("expected error for cross-tenant resource access, got nil")
	}
}

func TestCheckResourceAccessDeniedNonexistent(t *testing.T) {
	d, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer d.Close()
	if err := db.Seed(d); err != nil {
		t.Fatalf("Seed: %v", err)
	}

	access := makeTenantAccess(t, d, "u-alice", "t-acme")
	resourceID := shenguard.NewResourceId("r-nonexistent")

	_, err = CheckResourceAccess(d, access, resourceID)
	if err == nil {
		t.Fatal("expected error for nonexistent resource, got nil")
	}
}
