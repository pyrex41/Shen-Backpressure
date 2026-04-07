package main

import (
	"fmt"
	"log"
	"net/http"
)

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "dosage-calculator")
	})
	log.Println("listening on :8080")
	log.Fatal(http.ListenAndServe(":8080", mux))
}
