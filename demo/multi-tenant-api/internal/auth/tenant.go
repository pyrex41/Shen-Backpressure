package auth

import (
	"database/sql"
	"fmt"

	"multi-tenant-api/internal/shenguard"
)

// CheckTenantAccess verifies that the principal is a member of the given tenant
// and, if so, constructs a TenantAccess proof. The proof chain is:
// AuthenticatedPrincipal + tenant membership check → TenantAccess.
// For human principals, checks user_id in tenant_memberships.
// For service principals, checks service_id in tenant_memberships.
func CheckTenantAccess(db *sql.DB, principal shenguard.AuthenticatedPrincipal, userID string, tenantID shenguard.TenantId) (shenguard.TenantAccess, error) {
	var exists int
	err := db.QueryRow(
		"SELECT COUNT(*) FROM tenant_memberships WHERE user_id = ? AND tenant_id = ?",
		userID, tenantID.Val(),
	).Scan(&exists)
	if err != nil {
		return shenguard.TenantAccess{}, fmt.Errorf("check tenant membership: %w", err)
	}

	isMember := exists > 0
	access, err := shenguard.NewTenantAccess(principal, tenantID, isMember)
	if err != nil {
		return shenguard.TenantAccess{}, fmt.Errorf("tenant access denied: %s is not a member of tenant %s", userID, tenantID.Val())
	}
	return access, nil
}

// CheckResourceAccess verifies that the tenant owns the given resource and,
// if so, constructs a ResourceAccess proof. The proof chain is:
// TenantAccess + resource ownership check → ResourceAccess.
func CheckResourceAccess(db *sql.DB, access shenguard.TenantAccess, resourceID shenguard.ResourceId) (shenguard.ResourceAccess, error) {
	var exists int
	err := db.QueryRow(
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

// LogAccess records an access attempt in the access_logs table.
func LogAccess(db *sql.DB, userID, tenantID, resourceID, action string, allowed bool) error {
	allowedInt := 0
	if allowed {
		allowedInt = 1
	}
	_, err := db.Exec(
		"INSERT INTO access_logs (user_id, tenant_id, resource_id, action, allowed) VALUES (?, ?, ?, ?, ?)",
		userID, tenantID, resourceID, action, allowedInt,
	)
	return err
}
