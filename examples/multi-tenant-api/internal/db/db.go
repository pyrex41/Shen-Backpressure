package db

import (
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite"
)

const schema = `
CREATE TABLE IF NOT EXISTS tenants (
	id   TEXT PRIMARY KEY,
	name TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS users (
	id            TEXT PRIMARY KEY,
	email         TEXT NOT NULL UNIQUE,
	password_hash TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS tenant_memberships (
	user_id   TEXT NOT NULL REFERENCES users(id),
	tenant_id TEXT NOT NULL REFERENCES tenants(id),
	role      TEXT NOT NULL DEFAULT 'member',
	PRIMARY KEY (user_id, tenant_id)
);

CREATE TABLE IF NOT EXISTS resources (
	id        TEXT PRIMARY KEY,
	tenant_id TEXT NOT NULL REFERENCES tenants(id),
	title     TEXT NOT NULL,
	body      TEXT NOT NULL DEFAULT '',
	created_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS access_logs (
	id         INTEGER PRIMARY KEY AUTOINCREMENT,
	user_id    TEXT NOT NULL,
	tenant_id  TEXT NOT NULL,
	resource_id TEXT,
	action     TEXT NOT NULL,
	allowed    INTEGER NOT NULL,
	timestamp  TEXT NOT NULL DEFAULT (datetime('now'))
);
`

func Open(path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	db.SetMaxOpenConns(1) // SQLite is single-writer
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		db.Close()
		return nil, fmt.Errorf("set WAL mode: %w", err)
	}
	if _, err := db.Exec("PRAGMA foreign_keys=ON"); err != nil {
		db.Close()
		return nil, fmt.Errorf("enable foreign keys: %w", err)
	}
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("apply schema: %w", err)
	}
	return db, nil
}

func Seed(db *sql.DB) error {
	const seedSQL = `
INSERT OR IGNORE INTO tenants (id, name) VALUES
	('t-acme',    'Acme Corp'),
	('t-globex',  'Globex Inc');

INSERT OR IGNORE INTO users (id, email, password_hash) VALUES
	('u-alice', 'alice@acme.com',   '$plain$alice123'),
	('u-bob',   'bob@globex.com',   '$plain$bob456'),
	('u-carol', 'carol@acme.com',   '$plain$carol789');

INSERT OR IGNORE INTO tenant_memberships (user_id, tenant_id, role) VALUES
	('u-alice', 't-acme',   'admin'),
	('u-bob',   't-globex', 'member'),
	('u-carol', 't-acme',   'member');

INSERT OR IGNORE INTO resources (id, tenant_id, title, body) VALUES
	('r-1', 't-acme',   'Acme Roadmap',      'Q3 priorities...'),
	('r-2', 't-acme',   'Acme Budget',        'FY26 budget draft'),
	('r-3', 't-globex', 'Globex Launch Plan', 'Product launch checklist');
`
	_, err := db.Exec(seedSQL)
	return err
}
