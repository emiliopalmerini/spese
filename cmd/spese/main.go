package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	apphttp "spese/internal/http"
	ports "spese/internal/sheets"
	gsheet "spese/internal/sheets/google"
	mem "spese/internal/sheets/memory"
	"syscall"
	"time"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8081"
	}

	// Choose data backend (default: memory). Seed from ./data if present.
	backend := os.Getenv("DATA_BACKEND")
	if backend == "" {
		backend = "memory"
	}

    var (
        expWriter  ports.ExpenseWriter
        taxReader  ports.TaxonomyReader
        dashReader ports.DashboardReader
        expLister  ports.ExpenseLister
    )

	switch backend {
	case "sheets":
		cli, err := gsheet.NewFromEnv(context.Background())
		if err != nil {
			log.Fatalf("google sheets init: %v", err)
		}
        expWriter, taxReader, dashReader, expLister = cli, cli, cli, cli
	default:
		store := mem.NewFromFiles("data")
        expWriter, taxReader, dashReader, expLister = store, store, store, store
	}

    srv := apphttp.NewServer(":"+port, expWriter, taxReader, dashReader, expLister)

	// Configure server timeouts and limits
	srv.ReadTimeout = 10 * time.Second
	srv.WriteTimeout = 10 * time.Second
	srv.IdleTimeout = 60 * time.Second
	srv.MaxHeaderBytes = 1 << 16 // 64KB

	// Graceful shutdown handling
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
		<-sigChan
		log.Printf("shutdown signal received")

		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer shutdownCancel()

		if err := srv.Shutdown(shutdownCtx); err != nil {
			log.Printf("server shutdown error: %v", err)
		}
		cancel()
	}()

	log.Printf("starting spese on :%s", port)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("server error: %v", err)
	}

	<-ctx.Done()
	log.Printf("server stopped")
}
