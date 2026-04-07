package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"

	"multi-tenant-api/internal/db"
	"multi-tenant-api/internal/handlers"
)

func main() {
	addr := flag.String("addr", ":8080", "listen address")
	dbPath := flag.String("db", "data.db", "SQLite database path")
	seed := flag.Bool("seed", false, "seed database with demo data")
	flag.Parse()

	secret := []byte(os.Getenv("JWT_SECRET"))
	if len(secret) == 0 {
		secret = []byte("dev-secret-change-me")
	}

	database, err := db.Open(*dbPath)
	if err != nil {
		log.Fatalf("open database: %v", err)
	}
	defer database.Close()

	if *seed {
		if err := db.Seed(database); err != nil {
			log.Fatalf("seed database: %v", err)
		}
		log.Println("Database seeded with demo data")
	}

	srv := &handlers.Server{DB: database, Secret: secret}
	mux := http.NewServeMux()
	srv.Register(mux)

	log.Printf("Listening on %s", *addr)
	if err := http.ListenAndServe(*addr, mux); err != nil {
		fmt.Fprintf(os.Stderr, "server error: %v\n", err)
		os.Exit(1)
	}
}
