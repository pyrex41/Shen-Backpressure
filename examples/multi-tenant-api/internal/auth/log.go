package auth

import (
	"database/sql"
)

// LogAccess records an access attempt in the access_logs table.
//
// (The CheckTenantAccess / CheckResourceAccess wrappers that used to
// live here were moved to package internal/verified in W2.1 of the
// HN-follow-up plan. They derive the user-id directly from the
// principal — the redundant `userID string` parameter that the HN
// commenters singron / max_unbearable flagged is gone.)
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
