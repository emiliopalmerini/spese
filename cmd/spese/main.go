package main

import (
    "log"
    "net/http"
    "os"
    apphttp "spese/internal/http"
    mem "spese/internal/sheets/memory"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

    // Choose data backend (default: memory). Seed from ./data if present.
    _ = os.Getenv("DATA_BACKEND")
    store := mem.NewFromFiles("data")

    srv := apphttp.NewServer(":"+port, store, store)

	log.Printf("starting spese on :%s", port)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("server error: %v", err)
	}
}
