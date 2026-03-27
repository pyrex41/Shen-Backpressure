package db

import (
	"database/sql"
	"testing"
)

func mustOpen(t *testing.T) *sql.DB {
	t.Helper()
	d, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { d.Close() })
	return d
}

func TestOpenCreatesAllTables(t *testing.T) {
	d := mustOpen(t)

	tables := []string{"tenants", "users", "tenant_memberships", "resources", "access_logs"}
	for _, tbl := range tables {
		var name string
		err := d.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name=?", tbl).Scan(&name)
		if err != nil {
			t.Errorf("table %s not found: %v", tbl, err)
		}
	}
}

func TestSeedPopulatesData(t *testing.T) {
	d := mustOpen(t)
	if err := Seed(d); err != nil {
		t.Fatalf("Seed: %v", err)
	}

	var count int
	d.QueryRow("SELECT count(*) FROM tenants").Scan(&count)
	if count != 2 {
		t.Errorf("tenants: got %d, want 2", count)
	}

	d.QueryRow("SELECT count(*) FROM users").Scan(&count)
	if count != 3 {
		t.Errorf("users: got %d, want 3", count)
	}

	d.QueryRow("SELECT count(*) FROM tenant_memberships").Scan(&count)
	if count != 3 {
		t.Errorf("tenant_memberships: got %d, want 3", count)
	}

	d.QueryRow("SELECT count(*) FROM resources").Scan(&count)
	if count != 3 {
		t.Errorf("resources: got %d, want 3", count)
	}
}

func TestForeignKeysEnforced(t *testing.T) {
	d := mustOpen(t)

	_, err := d.Exec("INSERT INTO tenant_memberships (user_id, tenant_id, role) VALUES ('nonexistent', 'also-nonexistent', 'member')")
	if err == nil {
		t.Error("expected foreign key violation, got nil")
	}
}

func TestSeedIsIdempotent(t *testing.T) {
	d := mustOpen(t)
	if err := Seed(d); err != nil {
		t.Fatalf("first Seed: %v", err)
	}
	if err := Seed(d); err != nil {
		t.Fatalf("second Seed: %v", err)
	}

	var count int
	d.QueryRow("SELECT count(*) FROM users").Scan(&count)
	if count != 3 {
		t.Errorf("users after double seed: got %d, want 3", count)
	}
}
