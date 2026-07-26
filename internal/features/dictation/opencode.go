package dictation

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const extractionSystemPrompt = `Sei un parser di movimenti finanziari personali in italiano. Aggiorna l'intero elenco dei draft usando il nuovo segmento parlato e lo stato precedente. Interpreta correzioni come "anzi", "intendevo" e "togli quello" modificando o rimuovendo il draft pertinente. Non inventare conti, importi o beneficiari. Usa solo Income o Expense. Mantieni gli ID esistenti e lascia vuoto l'ID dei nuovi draft. Se l'utente dice che ha finito, imposta finish_requested. Restituisci esclusivamente l'output strutturato richiesto.`

// OpenCodeConfig identifies the local OpenCode server and extraction model.
type OpenCodeConfig struct {
	BaseURL    string
	Username   string
	Password   string
	ProviderID string
	ModelID    string
	Agent      string
}

// HistoryItem is the bounded accounting context supplied to the model.
type HistoryItem struct {
	Date        string `json:"date"`
	Kind        string `json:"kind"`
	Account     string `json:"account"`
	Amount      string `json:"amount"`
	Payee       string `json:"payee"`
	Category    string `json:"category"`
	Subcategory string `json:"subcategory"`
}

// ExtractRequest contains the current session state for one model turn.
type ExtractRequest struct {
	Today      time.Time     `json:"-"`
	Transcript string        `json:"transcript"`
	Previous   []Draft       `json:"previous"`
	Accounts   []string      `json:"accounts"`
	Categories []string      `json:"categories"`
	History    []HistoryItem `json:"recent_history"`
}

// OpenCodeClient calls a headless OpenCode server over HTTP.
type OpenCodeClient struct {
	config OpenCodeConfig
	http   *http.Client
}

func NewOpenCodeClient(config OpenCodeConfig, httpClient *http.Client) *OpenCodeClient {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	config.BaseURL = strings.TrimRight(strings.TrimSpace(config.BaseURL), "/")
	return &OpenCodeClient{config: config, http: httpClient}
}

func (c *OpenCodeClient) CreateSession(ctx context.Context) (string, error) {
	var response struct {
		ID string `json:"id"`
	}
	if err := c.doJSON(ctx, http.MethodPost, "/session", map[string]string{"title": "Spese dictation"}, &response); err != nil {
		return "", err
	}
	if strings.TrimSpace(response.ID) == "" {
		return "", fmt.Errorf("opencode: create session returned an empty id")
	}
	return response.ID, nil
}

func (c *OpenCodeClient) Extract(ctx context.Context, sessionID string, input ExtractRequest) (Extraction, error) {
	prompt, err := extractionPrompt(input)
	if err != nil {
		return Extraction{}, err
	}
	body := map[string]any{
		"agent":  c.config.Agent,
		"system": extractionSystemPrompt,
		"model": map[string]string{
			"providerID": c.config.ProviderID,
			"modelID":    c.config.ModelID,
		},
		"parts": []map[string]string{{"type": "text", "text": prompt}},
		"format": map[string]any{
			"type":       "json_schema",
			"schema":     extractionSchema(),
			"retryCount": 1,
		},
	}
	var response struct {
		Info struct {
			StructuredOutput json.RawMessage `json:"structured_output"`
			Error            json.RawMessage `json:"error"`
		} `json:"info"`
	}
	path := "/session/" + url.PathEscape(sessionID) + "/message"
	if err := c.doJSON(ctx, http.MethodPost, path, body, &response); err != nil {
		return Extraction{}, err
	}
	if len(response.Info.Error) > 0 && string(response.Info.Error) != "null" {
		return Extraction{}, fmt.Errorf("opencode: structured extraction failed: %s", response.Info.Error)
	}
	if len(response.Info.StructuredOutput) == 0 || string(response.Info.StructuredOutput) == "null" {
		return Extraction{}, fmt.Errorf("opencode: response did not contain structured output")
	}
	var extraction Extraction
	if err := json.Unmarshal(response.Info.StructuredOutput, &extraction); err != nil {
		return Extraction{}, fmt.Errorf("opencode: decode structured output: %w", err)
	}
	return NormalizeExtraction(input.Previous, extraction), nil
}

func (c *OpenCodeClient) DeleteSession(ctx context.Context, sessionID string) error {
	var deleted bool
	path := "/session/" + url.PathEscape(sessionID)
	if err := c.doJSON(ctx, http.MethodDelete, path, nil, &deleted); err != nil {
		return err
	}
	if !deleted {
		return fmt.Errorf("opencode: session %q was not deleted", sessionID)
	}
	return nil
}

func (c *OpenCodeClient) doJSON(ctx context.Context, method, path string, input, output any) error {
	if c.config.BaseURL == "" {
		return fmt.Errorf("opencode: base URL is required")
	}
	var body io.Reader
	if input != nil {
		encoded, err := json.Marshal(input)
		if err != nil {
			return fmt.Errorf("opencode: encode request: %w", err)
		}
		body = bytes.NewReader(encoded)
	}
	request, err := http.NewRequestWithContext(ctx, method, c.config.BaseURL+path, body)
	if err != nil {
		return fmt.Errorf("opencode: create request: %w", err)
	}
	request.Header.Set("Accept", "application/json")
	if input != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	if c.config.Username != "" || c.config.Password != "" {
		request.SetBasicAuth(c.config.Username, c.config.Password)
	}
	response, err := c.http.Do(request)
	if err != nil {
		return fmt.Errorf("opencode: request: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		detail, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
		return fmt.Errorf("opencode: %s: %s", response.Status, strings.TrimSpace(string(detail)))
	}
	if output == nil {
		return nil
	}
	if err := json.NewDecoder(response.Body).Decode(output); err != nil {
		return fmt.Errorf("opencode: decode response: %w", err)
	}
	return nil
}

func extractionPrompt(input ExtractRequest) (string, error) {
	payload := struct {
		Today      string        `json:"today"`
		Timezone   string        `json:"timezone"`
		Transcript string        `json:"new_transcript_segment"`
		Previous   []Draft       `json:"current_drafts"`
		Accounts   []string      `json:"allowed_accounts"`
		Categories []string      `json:"known_categories"`
		History    []HistoryItem `json:"recent_history"`
	}{
		Today: input.Today.Format("2006-01-02"), Timezone: "Europe/Rome",
		Transcript: input.Transcript, Previous: input.Previous, Accounts: input.Accounts,
		Categories: input.Categories, History: input.History,
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("opencode: encode extraction prompt: %w", err)
	}
	return string(encoded), nil
}

func extractionSchema() map[string]any {
	stringField := func(description string) map[string]any {
		return map[string]any{"type": "string", "description": description}
	}
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"movements", "finish_requested"},
		"properties": map[string]any{
			"movements": map[string]any{
				"type": "array", "maxItems": maxDrafts,
				"items": map[string]any{
					"type":                 "object",
					"additionalProperties": false,
					"required":             []string{"id", "kind", "date", "account", "amount", "payee", "category", "subcategory", "note", "issues"},
					"properties": map[string]any{
						"id":          stringField("ID esistente oppure stringa vuota per un nuovo movimento"),
						"kind":        map[string]any{"type": "string", "enum": []string{"Income", "Expense"}},
						"date":        stringField("Data ISO YYYY-MM-DD"),
						"account":     stringField("Nome esatto di un conto ammesso"),
						"amount":      stringField("Importo positivo in formato decimale, senza valuta"),
						"payee":       stringField("Beneficiario o descrizione"),
						"category":    stringField("Categoria, se nota"),
						"subcategory": stringField("Sottocategoria, se nota"),
						"note":        stringField("Nota facoltativa"),
						"issues":      map[string]any{"type": "array", "items": stringField("Ambiguita da verificare")},
					},
				},
			},
			"finish_requested": map[string]any{"type": "boolean"},
		},
	}
}
