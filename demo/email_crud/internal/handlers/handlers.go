package handlers

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"html/template"
	"net/http"
	"strconv"

	"email_crud/internal/db"
	"email_crud/internal/models"
)

type Server struct {
	DB   *sql.DB
	Tmpl *template.Template
}

func newID() string {
	b := make([]byte, 8)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// --- Dashboard ---

func (s *Server) HandleIndex(w http.ResponseWriter, r *http.Request) {
	campaigns, _ := db.ListCampaigns(s.DB)
	users, _ := db.ListUsers(s.DB)
	s.Tmpl.ExecuteTemplate(w, "index.html", map[string]any{
		"Campaigns": campaigns,
		"Users":     users,
	})
}

// --- Users CRUD ---

func (s *Server) HandleUsersList(w http.ResponseWriter, r *http.Request) {
	users, _ := db.ListUsers(s.DB)
	s.Tmpl.ExecuteTemplate(w, "users.html", map[string]any{
		"Users":     users,
		"States":    models.ValidStates,
		"Decades":   models.ValidAgeDecades,
	})
}

func (s *Server) HandleUsersCreate(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	u := &models.User{
		ID:    newID(),
		Email: r.FormValue("email"),
	}
	if ad := r.FormValue("age_decade"); ad != "" {
		v, _ := strconv.Atoi(ad)
		u.AgeDecade = &v
	}
	if st := r.FormValue("state"); st != "" {
		u.State = &st
	}
	db.CreateUser(s.DB, u)
	users, _ := db.ListUsers(s.DB)
	s.Tmpl.ExecuteTemplate(w, "user_rows", users)
}

func (s *Server) HandleUsersDelete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	db.DeleteUser(s.DB, id)
	w.WriteHeader(http.StatusOK)
}

func (s *Server) HandleUsersUpdate(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	r.ParseForm()
	u, err := db.GetUser(s.DB, id)
	if err != nil {
		http.Error(w, "user not found", http.StatusNotFound)
		return
	}
	u.Email = r.FormValue("email")
	if ad := r.FormValue("age_decade"); ad != "" {
		v, _ := strconv.Atoi(ad)
		u.AgeDecade = &v
	} else {
		u.AgeDecade = nil
	}
	if st := r.FormValue("state"); st != "" {
		u.State = &st
	} else {
		u.State = nil
	}
	db.UpdateUser(s.DB, u)
	users, _ := db.ListUsers(s.DB)
	s.Tmpl.ExecuteTemplate(w, "user_rows", users)
}

// --- Campaigns CRUD ---

func (s *Server) HandleCampaignsList(w http.ResponseWriter, r *http.Request) {
	campaigns, _ := db.ListCampaigns(s.DB)
	s.Tmpl.ExecuteTemplate(w, "campaigns.html", map[string]any{
		"Campaigns": campaigns,
	})
}

func (s *Server) HandleCampaignsCreate(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	c := &models.Campaign{
		ID:      newID(),
		Name:    r.FormValue("name"),
		Subject: r.FormValue("subject"),
		CtaURL:  r.FormValue("cta_url"),
	}
	db.CreateCampaign(s.DB, c)
	campaigns, _ := db.ListCampaigns(s.DB)
	s.Tmpl.ExecuteTemplate(w, "campaign_rows", campaigns)
}

func (s *Server) HandleCampaignsDelete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	db.DeleteCampaign(s.DB, id)
	w.WriteHeader(http.StatusOK)
}

// --- Copy Variants ---

func (s *Server) HandleCopyVariants(w http.ResponseWriter, r *http.Request) {
	campaignID := r.PathValue("campaign_id")
	campaign, err := db.GetCampaign(s.DB, campaignID)
	if err != nil {
		http.Error(w, "campaign not found", http.StatusNotFound)
		return
	}
	variants, _ := db.ListCopyVariants(s.DB, campaignID)
	s.Tmpl.ExecuteTemplate(w, "copy_variants.html", map[string]any{
		"Campaign": campaign,
		"Variants": variants,
		"States":   models.ValidStates,
		"Decades":  models.ValidAgeDecades,
	})
}

func (s *Server) HandleCopyVariantsCreate(w http.ResponseWriter, r *http.Request) {
	campaignID := r.PathValue("campaign_id")
	r.ParseForm()
	ad, _ := strconv.Atoi(r.FormValue("age_decade"))
	cv := &models.CopyVariant{
		ID:         newID(),
		CampaignID: campaignID,
		AgeDecade:  ad,
		State:      r.FormValue("state"),
		Body:       r.FormValue("body"),
	}
	db.CreateCopyVariant(s.DB, cv)
	variants, _ := db.ListCopyVariants(s.DB, campaignID)
	s.Tmpl.ExecuteTemplate(w, "variant_rows", variants)
}

func (s *Server) HandleCopyVariantsDelete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	db.DeleteCopyVariant(s.DB, id)
	w.WriteHeader(http.StatusOK)
}

// --- Send Email (simulated) ---

func (s *Server) HandleSendEmail(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	campaignID := r.FormValue("campaign_id")
	userID := r.FormValue("user_id")
	campaign, err := db.GetCampaign(s.DB, campaignID)
	if err != nil {
		http.Error(w, "campaign not found", http.StatusNotFound)
		return
	}
	user, err := db.GetUser(s.DB, userID)
	if err != nil {
		http.Error(w, "user not found", http.StatusNotFound)
		return
	}
	es := &models.EmailSend{
		ID:         newID(),
		CampaignID: campaignID,
		UserID:     userID,
	}
	db.CreateEmailSend(s.DB, es)
	s.Tmpl.ExecuteTemplate(w, "send_result", map[string]any{
		"Campaign": campaign,
		"User":     user,
		"SendID":   es.ID,
	})
}

// --- CTA Landing Page ---
// This is the core flow enforcing the Shen invariant:
// known-profile -> show tailored copy
// unknown-profile -> prompt for demographics first

func (s *Server) HandleCTALanding(w http.ResponseWriter, r *http.Request) {
	campaignID := r.PathValue("campaign_id")
	userID := r.PathValue("user_id")

	campaign, err := db.GetCampaign(s.DB, campaignID)
	if err != nil {
		http.Error(w, "campaign not found", http.StatusNotFound)
		return
	}
	user, err := db.GetUser(s.DB, userID)
	if err != nil {
		http.Error(w, "user not found", http.StatusNotFound)
		return
	}

	// Shen invariant: copy-delivery requires known-profile
	if !user.IsKnown() {
		// unknown-profile -> prompt-required
		s.Tmpl.ExecuteTemplate(w, "cta_prompt.html", map[string]any{
			"Campaign": campaign,
			"User":     user,
			"States":   models.ValidStates,
			"Decades":  models.ValidAgeDecades,
		})
		return
	}

	// known-profile -> look up copy-content matching demographics
	variant, err := db.GetCopyVariant(s.DB, campaignID, *user.AgeDecade, *user.State)
	if err != nil {
		// No variant for this demographic - show fallback
		s.Tmpl.ExecuteTemplate(w, "cta_landing.html", map[string]any{
			"Campaign": campaign,
			"User":     user,
			"Body":     fmt.Sprintf("Welcome, %s! We don't have custom content for your demographic yet, but stay tuned.", user.Email),
		})
		return
	}

	// safe-copy-view: delivery is valid
	s.Tmpl.ExecuteTemplate(w, "cta_landing.html", map[string]any{
		"Campaign": campaign,
		"User":     user,
		"Body":     variant.Body,
	})
}

// HandleCTAPromptSubmit upgrades an unknown-profile to known-profile
// (Shen: profile-upgrade), then redirects to show tailored copy.
func (s *Server) HandleCTAPromptSubmit(w http.ResponseWriter, r *http.Request) {
	campaignID := r.PathValue("campaign_id")
	userID := r.PathValue("user_id")
	r.ParseForm()

	user, err := db.GetUser(s.DB, userID)
	if err != nil {
		http.Error(w, "user not found", http.StatusNotFound)
		return
	}

	ad, _ := strconv.Atoi(r.FormValue("age_decade"))
	st := r.FormValue("state")
	user.AgeDecade = &ad
	user.State = &st
	db.UpdateUser(s.DB, user)

	// Now the user is a known-profile — redirect to CTA landing
	http.Redirect(w, r, fmt.Sprintf("/cta/%s/%s", campaignID, userID), http.StatusSeeOther)
}
