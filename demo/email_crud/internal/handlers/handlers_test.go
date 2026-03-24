package handlers

import (
	"html/template"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"email_crud/internal/db"
	"email_crud/internal/models"
)

func setupTestServer(t *testing.T) *Server {
	t.Helper()
	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	funcMap := template.FuncMap{
		"derefInt": func(p *int) int {
			if p == nil {
				return 0
			}
			return *p
		},
		"derefStr": func(p *string) string {
			if p == nil {
				return ""
			}
			return *p
		},
	}
	tmpl := template.Must(template.New("").Funcs(funcMap).ParseGlob("../../templates/*.html"))

	return &Server{DB: database, Tmpl: tmpl}
}

func seedCampaignAndVariant(t *testing.T, srv *Server) (*models.Campaign, *models.CopyVariant) {
	t.Helper()
	c := &models.Campaign{ID: "camp1", Name: "Test Campaign", Subject: "Hello", CtaURL: "/cta/camp1"}
	if err := db.CreateCampaign(srv.DB, c); err != nil {
		t.Fatalf("create campaign: %v", err)
	}
	cv := &models.CopyVariant{ID: "cv1", CampaignID: "camp1", AgeDecade: 30, State: "CA", Body: "Tailored for 30s in CA"}
	if err := db.CreateCopyVariant(srv.DB, cv); err != nil {
		t.Fatalf("create copy variant: %v", err)
	}
	return c, cv
}

func TestCTALanding_KnownUserSeesCopy(t *testing.T) {
	srv := setupTestServer(t)
	seedCampaignAndVariant(t, srv)

	age := 30
	state := "CA"
	u := &models.User{ID: "u1", Email: "known@test.com", AgeDecade: &age, State: &state}
	db.CreateUser(srv.DB, u)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /cta/{campaign_id}/{user_id}", srv.HandleCTALanding)

	req := httptest.NewRequest("GET", "/cta/camp1/u1", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "Tailored for 30s in CA") {
		t.Errorf("expected tailored copy in response, got: %s", body)
	}
}

func TestCTALanding_UnknownUserGetsPrompt(t *testing.T) {
	srv := setupTestServer(t)
	seedCampaignAndVariant(t, srv)

	u := &models.User{ID: "u2", Email: "unknown@test.com"}
	db.CreateUser(srv.DB, u)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /cta/{campaign_id}/{user_id}", srv.HandleCTALanding)

	req := httptest.NewRequest("GET", "/cta/camp1/u2", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "tell us a bit about yourself") {
		t.Errorf("expected demographics prompt in response, got: %s", body)
	}
}

func TestCTAPromptSubmit_UpgradesProfileAndRedirects(t *testing.T) {
	srv := setupTestServer(t)
	seedCampaignAndVariant(t, srv)

	u := &models.User{ID: "u3", Email: "upgrading@test.com"}
	db.CreateUser(srv.DB, u)

	mux := http.NewServeMux()
	mux.HandleFunc("POST /cta/{campaign_id}/{user_id}", srv.HandleCTAPromptSubmit)

	form := url.Values{}
	form.Set("age_decade", "30")
	form.Set("state", "CA")
	req := httptest.NewRequest("POST", "/cta/camp1/u3", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	// Should redirect (303 See Other) to the CTA landing
	if w.Code != http.StatusSeeOther {
		t.Fatalf("expected 303 redirect, got %d", w.Code)
	}
	loc := w.Header().Get("Location")
	if loc != "/cta/camp1/u3" {
		t.Errorf("expected redirect to /cta/camp1/u3, got %s", loc)
	}

	// Verify user is now known
	updated, err := db.GetUser(srv.DB, "u3")
	if err != nil {
		t.Fatalf("get user: %v", err)
	}
	if !updated.IsKnown() {
		t.Error("expected user to be known after prompt submit")
	}
	if *updated.AgeDecade != 30 {
		t.Errorf("expected age_decade 30, got %d", *updated.AgeDecade)
	}
	if *updated.State != "CA" {
		t.Errorf("expected state CA, got %s", *updated.State)
	}
}
