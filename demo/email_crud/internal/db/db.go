package db

import (
	"database/sql"

	_ "github.com/mattn/go-sqlite3"
)

func Open(path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite3", path+"?_journal_mode=WAL&_foreign_keys=on")
	if err != nil {
		return nil, err
	}
	if err := migrate(db); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

func migrate(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS users (
			id         TEXT PRIMARY KEY,
			email      TEXT NOT NULL UNIQUE,
			age_decade INTEGER,
			state      TEXT,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);

		CREATE TABLE IF NOT EXISTS campaigns (
			id         TEXT PRIMARY KEY,
			name       TEXT NOT NULL,
			subject    TEXT NOT NULL,
			cta_url    TEXT NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);

		CREATE TABLE IF NOT EXISTS copy_variants (
			id          TEXT PRIMARY KEY,
			campaign_id TEXT NOT NULL REFERENCES campaigns(id) ON DELETE CASCADE,
			age_decade  INTEGER NOT NULL,
			state       TEXT NOT NULL,
			body        TEXT NOT NULL,
			UNIQUE(campaign_id, age_decade, state)
		);

		CREATE TABLE IF NOT EXISTS email_sends (
			id          TEXT PRIMARY KEY,
			campaign_id TEXT NOT NULL REFERENCES campaigns(id),
			user_id     TEXT NOT NULL REFERENCES users(id),
			sent_at     DATETIME DEFAULT CURRENT_TIMESTAMP
		);
	`)
	return err
}
