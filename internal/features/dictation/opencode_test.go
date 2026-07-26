package dictation

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestOpenCodeClientSessionLifecycleAndExtraction(t *testing.T) {
	t.Parallel()

	deleted := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		username, password, ok := r.BasicAuth()
		if !ok || username != "voice" || password != "secret" {
			t.Errorf("basic auth = %q/%q/%v", username, password, ok)
		}
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/session":
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "session-1"})
		case r.Method == http.MethodPost && r.URL.Path == "/session/session-1/message":
			var body struct {
				Agent  string `json:"agent"`
				System string `json:"system"`
				Model  struct {
					ProviderID string `json:"providerID"`
					ModelID    string `json:"modelID"`
				} `json:"model"`
				Format struct {
					Type   string         `json:"type"`
					Schema map[string]any `json:"schema"`
				} `json:"format"`
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			if body.Agent != "dictation" || body.Model.ProviderID != "ollama" || body.Model.ModelID != "qwen3" {
				t.Errorf("routing = %#v", body)
			}
			if body.Format.Type != "json_schema" || body.Format.Schema["type"] != "object" {
				t.Errorf("format = %#v", body.Format)
			}
			if !strings.Contains(body.System, "Valorizza sempre tutti i campi richiesti dallo schema") ||
				!strings.Contains(body.System, "Non rispondere con testo libero") {
				t.Errorf("system prompt does not enforce structured output: %q", body.System)
			}
			if len(body.Parts) != 1 || body.Parts[0].Text == "" {
				t.Errorf("parts = %#v", body.Parts)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"info": map[string]any{
					"structured": map[string]any{
						"movements": []map[string]any{{
							"id": "", "kind": "Expense", "date": "2026-07-26", "account": "Fineco",
							"amount": "12,50", "payee": "Conad", "category": "Spesa",
							"subcategory": "", "note": "", "issues": []string{},
						}},
						"finish_requested": false,
					},
				},
			})
		case r.Method == http.MethodDelete && r.URL.Path == "/session/session-1":
			deleted = true
			_ = json.NewEncoder(w).Encode(true)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := NewOpenCodeClient(OpenCodeConfig{
		BaseURL: server.URL, Username: "voice", Password: "secret",
		ProviderID: "ollama", ModelID: "qwen3", Agent: "dictation",
	}, server.Client())
	sessionID, err := client.CreateSession(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	got, err := client.Extract(context.Background(), sessionID, ExtractRequest{
		Today:      time.Date(2026, 7, 26, 0, 0, 0, 0, time.UTC),
		Transcript: "Dodici euro e cinquanta da Conad con Fineco",
		Accounts:   []string{"Fineco"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Movements) != 1 || got.Movements[0].Payee != "Conad" {
		t.Fatalf("extraction = %#v", got)
	}
	if err := client.DeleteSession(context.Background(), sessionID); err != nil {
		t.Fatal(err)
	}
	if !deleted {
		t.Fatal("session was not deleted")
	}
}

func TestOpenCodeClientReportsAPIError(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "model unavailable", http.StatusServiceUnavailable)
	}))
	defer server.Close()

	client := NewOpenCodeClient(OpenCodeConfig{BaseURL: server.URL}, server.Client())
	if _, err := client.CreateSession(context.Background()); err == nil {
		t.Fatal("CreateSession() error = nil")
	}
}
