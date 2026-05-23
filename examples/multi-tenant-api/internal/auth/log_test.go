package auth

import (
	"testing"

	"multi-tenant-api/internal/db"
)

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
