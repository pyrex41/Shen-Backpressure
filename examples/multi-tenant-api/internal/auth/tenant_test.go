package auth

import (
	"database/sql"
	"testing"

	"multi-tenant-api/internal/db"
	"multi-tenant-api/internal/shenguard"
)

func makePrincipal(t *testing.T, userID string) (shenguard.HumanPrincipal, string) {
	t.Helper()
	tok, err := shenguard.NewJwtToken("test-token")
	if err != nil {
		t.Fatalf("NewJwtToken: %v", err)
	}
	exp, err := shenguard.NewTokenExpiry(9999999999, 1000000000)
	if err != nil {
		t.Fatalf("NewTokenExpiry: %v", err)
	}
	authUser := shenguard.NewAuthenticatedUser(tok, exp, shenguard.NewUserId(userID))
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

	principal, userID := makePrincipal(t, "u-alice")
	tenantID := shenguard.NewTenantId("t-acme")

	access, err := CheckTenantAccess(d, principal, userID, tenantID)
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
	principal, userID := makePrincipal(t, "u-bob")
	tenantID := shenguard.NewTenantId("t-acme")

	_, err = CheckTenantAccess(d, principal, userID, tenantID)
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

	principal, userID := makePrincipal(t, "u-nobody")
	tenantID := shenguard.NewTenantId("t-acme")

	_, err = CheckTenantAccess(d, principal, userID, tenantID)
	if err == nil {
		t.Fatal("expected error for nonexistent user, got nil")
	}
}

func makeTenantAccess(t *testing.T, d *sql.DB, userID, tenantID string) shenguard.TenantAccess {
	t.Helper()
	principal, uid := makePrincipal(t, userID)
	tid := shenguard.NewTenantId(tenantID)
	access, err := CheckTenantAccess(d, principal, uid, tid)
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

func TestLogAccess(t *testing.T) {
	d, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer d.Close()
	if err := db.Seed(d); err != nil {
		t.Fatalf("Seed: %v", err)
	}

	if err := LogAccess(d, "u-alice", "t-acme", "r-1", "read", true); err != nil {
		t.Fatalf("LogAccess: %v", err)
	}
	if err := LogAccess(d, "u-bob", "t-acme", "", "list", false); err != nil {
		t.Fatalf("LogAccess denied: %v", err)
	}

	var count int
	d.QueryRow("SELECT count(*) FROM access_logs").Scan(&count)
	if count != 2 {
		t.Errorf("access_logs: got %d, want 2", count)
	}

	var allowed int
	d.QueryRow("SELECT allowed FROM access_logs WHERE user_id='u-bob'").Scan(&allowed)
	if allowed != 0 {
		t.Errorf("denied log: got allowed=%d, want 0", allowed)
	}
}
