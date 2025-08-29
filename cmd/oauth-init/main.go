package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/sheets/v4"
)

func main() {
	// Load client credentials
	clientJSON := os.Getenv("GOOGLE_OAUTH_CLIENT_JSON")
	clientFile := os.Getenv("GOOGLE_OAUTH_CLIENT_FILE")
	var b []byte
	var err error
	switch {
	case clientJSON != "":
		b = []byte(clientJSON)
	case clientFile != "":
		b, err = os.ReadFile(clientFile)
		if err != nil {
			log.Fatalf("read client file: %v", err)
		}
	default:
		log.Fatalf("set GOOGLE_OAUTH_CLIENT_JSON or GOOGLE_OAUTH_CLIENT_FILE")
	}

	cfg, err := google.ConfigFromJSON(b, sheets.SpreadsheetsScope)
	if err != nil {
		log.Fatalf("oauth config: %v", err)
	}

	// Start local server for redirect_uri http://localhost:8085/callback
	// Update the OAuth client to include this URI in authorized redirect URIs.
	redirectPort := os.Getenv("OAUTH_REDIRECT_PORT")
	if redirectPort == "" {
		redirectPort = "8085"
	}
	redirectURL := "http://localhost:" + redirectPort + "/callback"
	cfg.RedirectURL = redirectURL

	codeCh := make(chan string, 1)
	srv := &http.Server{Addr: ":" + redirectPort}
	http.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		if errStr := r.URL.Query().Get("error"); errStr != "" {
			http.Error(w, "OAuth error: "+errStr, http.StatusBadRequest)
			return
		}
		code := r.URL.Query().Get("code")
		fmt.Fprintln(w, "You may close this window and return to the terminal.")
		codeCh <- code
		go func() { time.Sleep(500 * time.Millisecond); _ = srv.Close() }()
	})
	go func() { _ = srv.ListenAndServe() }()

	url := cfg.AuthCodeURL("state-token", oauth2.AccessTypeOffline)
	fmt.Printf("Open this URL to authorize:\n%s\n", url)

	// Wait for code
	select {
	case code := <-codeCh:
		tok, err := cfg.Exchange(context.Background(), code)
		if err != nil {
			log.Fatalf("token exchange: %v", err)
		}
		// Save token
		outFile := os.Getenv("GOOGLE_OAUTH_TOKEN_FILE")
		if outFile == "" {
			outFile = "token.json"
		}
		f, err := os.OpenFile(outFile, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
		if err != nil {
			log.Fatalf("open token file: %v", err)
		}
		defer f.Close()
		if err := json.NewEncoder(f).Encode(tok); err != nil {
			log.Fatalf("write token: %v", err)
		}
		fmt.Printf("Saved token to %s\n", outFile)
	case <-time.After(5 * time.Minute):
		log.Fatalf("authorization timed out")
	case <-signalChan():
		log.Fatalf("interrupted")
	}
}

func signalChan() <-chan os.Signal {
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	return c
}
