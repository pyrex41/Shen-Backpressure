package db

import (
	"database/sql"
	"email_crud/internal/models"
)

// --- Users ---

func CreateUser(db *sql.DB, u *models.User) error {
	_, err := db.Exec(`INSERT INTO users (id, email, age_decade, state) VALUES (?, ?, ?, ?)`,
		u.ID, u.Email, u.AgeDecade, u.State)
	return err
}

func GetUser(db *sql.DB, id string) (*models.User, error) {
	u := &models.User{}
	err := db.QueryRow(`SELECT id, email, age_decade, state, created_at FROM users WHERE id = ?`, id).
		Scan(&u.ID, &u.Email, &u.AgeDecade, &u.State, &u.CreatedAt)
	if err != nil {
		return nil, err
	}
	return u, nil
}

func GetUserByEmail(db *sql.DB, email string) (*models.User, error) {
	u := &models.User{}
	err := db.QueryRow(`SELECT id, email, age_decade, state, created_at FROM users WHERE email = ?`, email).
		Scan(&u.ID, &u.Email, &u.AgeDecade, &u.State, &u.CreatedAt)
	if err != nil {
		return nil, err
	}
	return u, nil
}

func ListUsers(db *sql.DB) ([]models.User, error) {
	rows, err := db.Query(`SELECT id, email, age_decade, state, created_at FROM users ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var users []models.User
	for rows.Next() {
		var u models.User
		if err := rows.Scan(&u.ID, &u.Email, &u.AgeDecade, &u.State, &u.CreatedAt); err != nil {
			return nil, err
		}
		users = append(users, u)
	}
	return users, rows.Err()
}

func UpdateUser(db *sql.DB, u *models.User) error {
	_, err := db.Exec(`UPDATE users SET email = ?, age_decade = ?, state = ? WHERE id = ?`,
		u.Email, u.AgeDecade, u.State, u.ID)
	return err
}

func DeleteUser(db *sql.DB, id string) error {
	_, err := db.Exec(`DELETE FROM users WHERE id = ?`, id)
	return err
}

// --- Campaigns ---

func CreateCampaign(db *sql.DB, c *models.Campaign) error {
	_, err := db.Exec(`INSERT INTO campaigns (id, name, subject, cta_url) VALUES (?, ?, ?, ?)`,
		c.ID, c.Name, c.Subject, c.CtaURL)
	return err
}

func GetCampaign(db *sql.DB, id string) (*models.Campaign, error) {
	c := &models.Campaign{}
	err := db.QueryRow(`SELECT id, name, subject, cta_url, created_at FROM campaigns WHERE id = ?`, id).
		Scan(&c.ID, &c.Name, &c.Subject, &c.CtaURL, &c.CreatedAt)
	if err != nil {
		return nil, err
	}
	return c, nil
}

func ListCampaigns(db *sql.DB) ([]models.Campaign, error) {
	rows, err := db.Query(`SELECT id, name, subject, cta_url, created_at FROM campaigns ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var campaigns []models.Campaign
	for rows.Next() {
		var c models.Campaign
		if err := rows.Scan(&c.ID, &c.Name, &c.Subject, &c.CtaURL, &c.CreatedAt); err != nil {
			return nil, err
		}
		campaigns = append(campaigns, c)
	}
	return campaigns, rows.Err()
}

func UpdateCampaign(db *sql.DB, c *models.Campaign) error {
	_, err := db.Exec(`UPDATE campaigns SET name = ?, subject = ?, cta_url = ? WHERE id = ?`,
		c.Name, c.Subject, c.CtaURL, c.ID)
	return err
}

func DeleteCampaign(db *sql.DB, id string) error {
	_, err := db.Exec(`DELETE FROM campaigns WHERE id = ?`, id)
	return err
}

// --- Copy Variants ---

func CreateCopyVariant(db *sql.DB, cv *models.CopyVariant) error {
	_, err := db.Exec(`INSERT INTO copy_variants (id, campaign_id, age_decade, state, body) VALUES (?, ?, ?, ?, ?)`,
		cv.ID, cv.CampaignID, cv.AgeDecade, cv.State, cv.Body)
	return err
}

func GetCopyVariant(db *sql.DB, campaignID string, ageDecade int, state string) (*models.CopyVariant, error) {
	cv := &models.CopyVariant{}
	err := db.QueryRow(
		`SELECT id, campaign_id, age_decade, state, body FROM copy_variants WHERE campaign_id = ? AND age_decade = ? AND state = ?`,
		campaignID, ageDecade, state).
		Scan(&cv.ID, &cv.CampaignID, &cv.AgeDecade, &cv.State, &cv.Body)
	if err != nil {
		return nil, err
	}
	return cv, nil
}

func ListCopyVariants(db *sql.DB, campaignID string) ([]models.CopyVariant, error) {
	rows, err := db.Query(
		`SELECT id, campaign_id, age_decade, state, body FROM copy_variants WHERE campaign_id = ? ORDER BY age_decade, state`,
		campaignID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var variants []models.CopyVariant
	for rows.Next() {
		var cv models.CopyVariant
		if err := rows.Scan(&cv.ID, &cv.CampaignID, &cv.AgeDecade, &cv.State, &cv.Body); err != nil {
			return nil, err
		}
		variants = append(variants, cv)
	}
	return variants, rows.Err()
}

func DeleteCopyVariant(db *sql.DB, id string) error {
	_, err := db.Exec(`DELETE FROM copy_variants WHERE id = ?`, id)
	return err
}

// --- Email Sends ---

func CreateEmailSend(db *sql.DB, es *models.EmailSend) error {
	_, err := db.Exec(`INSERT INTO email_sends (id, campaign_id, user_id) VALUES (?, ?, ?)`,
		es.ID, es.CampaignID, es.UserID)
	return err
}

func ListEmailSends(db *sql.DB, campaignID string) ([]models.EmailSend, error) {
	rows, err := db.Query(
		`SELECT id, campaign_id, user_id, sent_at FROM email_sends WHERE campaign_id = ? ORDER BY sent_at DESC`,
		campaignID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var sends []models.EmailSend
	for rows.Next() {
		var es models.EmailSend
		if err := rows.Scan(&es.ID, &es.CampaignID, &es.UserID, &es.SentAt); err != nil {
			return nil, err
		}
		sends = append(sends, es)
	}
	return sends, rows.Err()
}
