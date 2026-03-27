package auth

import (
	"database/sql"
	"fmt"

	"multi-tenant-api/internal/shenguard"
)

// CheckTenantAccess verifies that the authenticated user is a member of the
// given tenant and, if so, constructs a TenantAccess proof. The proof chain
// is: AuthenticatedUser + tenant membership check → TenantAccess.
func CheckTenantAccess(db *sql.DB, authUser shenguard.AuthenticatedUser, tenantID shenguard.TenantId) (shenguard.TenantAccess, error) {
	var exists int
	err := db.QueryRow(
		"SELECT COUNT(*) FROM tenant_memberships WHERE user_id = ? AND tenant_id = ?",
		authUser.User().Val(), tenantID.Val(),
	).Scan(&exists)
	if err != nil {
		return shenguard.TenantAccess{}, fmt.Errorf("check tenant membership: %w", err)
	}

	isMember := exists > 0
	access, err := shenguard.NewTenantAccess(authUser, tenantID, isMember)
	if err != nil {
		return shenguard.TenantAccess{}, fmt.Errorf("tenant access denied: user %s is not a member of tenant %s", authUser.User().Val(), tenantID.Val())
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
