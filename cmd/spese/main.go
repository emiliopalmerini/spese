package main

import (
    "context"
    "log"
    "net/http"
    "os"
    apphttp "spese/internal/http"
    ports "spese/internal/sheets"
    gsheet "spese/internal/sheets/google"
    mem "spese/internal/sheets/memory"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

    // Choose data backend (default: memory). Seed from ./data if present.
    backend := os.Getenv("DATA_BACKEND")
    if backend == "" {
        backend = "memory"
    }

    var (
        expWriter ports.ExpenseWriter
        taxReader ports.TaxonomyReader
    )

    switch backend {
    case "sheets":
        cli, err := gsheet.NewFromEnv(context.Background())
        if err != nil {
            log.Fatalf("google sheets init: %v", err)
        }
        expWriter, taxReader = cli, cli
    default:
        store := mem.NewFromFiles("data")
        expWriter, taxReader = store, store
    }

    srv := apphttp.NewServer(":"+port, expWriter, taxReader)

	log.Printf("starting spese on :%s", port)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("server error: %v", err)
	}
}
