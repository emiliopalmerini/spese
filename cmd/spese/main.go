package main

import (
    "log"
    "net/http"
    "os"
    apphttp "spese/internal/http"
)

func main() {
    port := os.Getenv("PORT")
    if port == "" {
        port = "8080"
    }

    srv := apphttp.NewServer(":" + port)

    log.Printf("starting spese on :%s", port)
    if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
        log.Fatalf("server error: %v", err)
    }
}

