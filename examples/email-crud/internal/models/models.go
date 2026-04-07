package models

import "time"

type User struct {
	ID        string `json:"id"`
	Email     string `json:"email"`
	AgeDecade *int   `json:"age_decade,omitempty"` // nil = unknown
	State     *string `json:"state,omitempty"`      // nil = unknown
	CreatedAt time.Time `json:"created_at"`
}

// IsKnown returns true if the user has both age_decade and state set.
// This maps to the Shen known-profile vs unknown-profile distinction.
func (u *User) IsKnown() bool {
	return u.AgeDecade != nil && u.State != nil
}

type Campaign struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Subject   string    `json:"subject"`
	CtaURL    string    `json:"cta_url"`
	CreatedAt time.Time `json:"created_at"`
}

type CopyVariant struct {
	ID         string `json:"id"`
	CampaignID string `json:"campaign_id"`
	AgeDecade  int    `json:"age_decade"`
	State      string `json:"state"`
	Body       string `json:"body"`
}

type EmailSend struct {
	ID         string    `json:"id"`
	CampaignID string    `json:"campaign_id"`
	UserID     string    `json:"user_id"`
	SentAt     time.Time `json:"sent_at"`
}

// ValidAgeDacades returns the allowed decade values (maps to Shen age-decade constraint).
var ValidAgeDecades = []int{10, 20, 30, 40, 50, 60, 70, 80, 90, 100}

// ValidStates is the list of US state codes (maps to Shen us-state constraint).
var ValidStates = []string{
	"AL", "AK", "AZ", "AR", "CA", "CO", "CT", "DE", "FL", "GA",
	"HI", "ID", "IL", "IN", "IA", "KS", "KY", "LA", "ME", "MD",
	"MA", "MI", "MN", "MS", "MO", "MT", "NE", "NV", "NH", "NJ",
	"NM", "NY", "NC", "ND", "OH", "OK", "OR", "PA", "RI", "SC",
	"SD", "TN", "TX", "UT", "VT", "VA", "WA", "WV", "WI", "WY",
	"DC",
}
