package main

import (
	"flag"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"

	"email_crud/internal/db"
	"email_crud/internal/handlers"
)

func main() {
	seed := flag.Bool("seed", false, "Seed the database with sample data")
	flag.Parse()

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	dbPath := os.Getenv("DB_PATH")
	if dbPath == "" {
		dbPath = "data.db"
	}

	database, err := db.Open(dbPath)
	if err != nil {
		log.Fatalf("failed to open database: %v", err)
	}
	defer database.Close()

	if *seed {
		if err := db.Seed(database); err != nil {
			log.Fatalf("failed to seed database: %v", err)
		}
		log.Println("Database seeded with sample data")
	}

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
	tmpl := template.Must(template.New("").Funcs(funcMap).ParseGlob("templates/*.html"))

	srv := &handlers.Server{
		DB:   database,
		Tmpl: tmpl,
	}

	mux := http.NewServeMux()

	// Static files
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))

	// Dashboard
	mux.HandleFunc("GET /", srv.HandleIndex)

	// Users CRUD
	mux.HandleFunc("GET /users", srv.HandleUsersList)
	mux.HandleFunc("POST /users", srv.HandleUsersCreate)
	mux.HandleFunc("PUT /users/{id}", srv.HandleUsersUpdate)
	mux.HandleFunc("DELETE /users/{id}", srv.HandleUsersDelete)

	// Campaigns CRUD
	mux.HandleFunc("GET /campaigns", srv.HandleCampaignsList)
	mux.HandleFunc("POST /campaigns", srv.HandleCampaignsCreate)
	mux.HandleFunc("PUT /campaigns/{id}", srv.HandleCampaignsUpdate)
	mux.HandleFunc("DELETE /campaigns/{id}", srv.HandleCampaignsDelete)

	// Copy Variants
	mux.HandleFunc("GET /campaigns/{campaign_id}/copy", srv.HandleCopyVariants)
	mux.HandleFunc("POST /campaigns/{campaign_id}/copy", srv.HandleCopyVariantsCreate)
	mux.HandleFunc("DELETE /copy/{id}", srv.HandleCopyVariantsDelete)

	// Send Email
	mux.HandleFunc("GET /send", srv.HandleSendPage)
	mux.HandleFunc("POST /send", srv.HandleSendEmail)

	// CTA Landing (the core backpressure-enforced flow)
	mux.HandleFunc("GET /cta/{campaign_id}/{user_id}", srv.HandleCTALanding)
	mux.HandleFunc("POST /cta/{campaign_id}/{user_id}", srv.HandleCTAPromptSubmit)

	addr := fmt.Sprintf(":%s", port)
	log.Printf("Server starting on %s", addr)
	log.Fatal(http.ListenAndServe(addr, mux))
}
