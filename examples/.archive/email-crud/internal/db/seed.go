package db

import "database/sql"

// Seed inserts sample data if the database is empty.
// Uses INSERT OR IGNORE so it's idempotent.
func Seed(db *sql.DB) error {
	var count int
	db.QueryRow(`SELECT COUNT(*) FROM users`).Scan(&count)
	if count > 0 {
		return nil // already seeded
	}

	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Sample users: 3 known (have demographics), 2 unknown
	users := []struct {
		id, email    string
		ageDecade    *int
		state        *string
	}{
		{"seed-u1", "alice@example.com", intPtr(30), strPtr("CA")},
		{"seed-u2", "bob@example.com", intPtr(50), strPtr("TX")},
		{"seed-u3", "carol@example.com", intPtr(20), strPtr("NY")},
		{"seed-u4", "dave@example.com", nil, nil},
		{"seed-u5", "eve@example.com", nil, nil},
	}
	for _, u := range users {
		_, err := tx.Exec(`INSERT OR IGNORE INTO users (id, email, age_decade, state) VALUES (?, ?, ?, ?)`,
			u.id, u.email, u.ageDecade, u.state)
		if err != nil {
			return err
		}
	}

	// Sample campaign
	_, err = tx.Exec(`INSERT OR IGNORE INTO campaigns (id, name, subject, cta_url) VALUES (?, ?, ?, ?)`,
		"seed-c1", "Spring Sale 2026", "Don't miss our spring deals!", "/cta/seed-c1")
	if err != nil {
		return err
	}

	// Copy variants for a few (age_decade, state) combos
	variants := []struct {
		id, campaignID string
		ageDecade      int
		state, body    string
	}{
		{"seed-cv1", "seed-c1", 30, "CA", "Hey California! Your 30s are the perfect time to upgrade your wardrobe. Check out our West Coast spring collection."},
		{"seed-cv2", "seed-c1", 50, "TX", "Howdy Texas! Our spring sale has everything for the discerning Texan in their 50s. Big savings, big style."},
		{"seed-cv3", "seed-c1", 20, "NY", "New York, New You! Spring is here and so are deals perfect for twenty-somethings in the city."},
		{"seed-cv4", "seed-c1", 30, "TX", "Texas meets thirty! Our spring lineup has bold styles for your bold decade. Shop now."},
		{"seed-cv5", "seed-c1", 20, "CA", "California dreaming in your 20s? Our spring collection was made for you. Sun, surf, and savings."},
	}
	for _, v := range variants {
		_, err := tx.Exec(`INSERT OR IGNORE INTO copy_variants (id, campaign_id, age_decade, state, body) VALUES (?, ?, ?, ?, ?)`,
			v.id, v.campaignID, v.ageDecade, v.state, v.body)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

func intPtr(i int) *int    { return &i }
func strPtr(s string) *string { return &s }
