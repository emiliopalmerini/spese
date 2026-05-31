package sheets

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestFileClientReplaceRowsPreservesOtherTabs(t *testing.T) {
	path := filepath.Join(t.TempDir(), "local-sheet.json")
	client, err := NewFileClient(path)
	if err != nil {
		t.Fatalf("NewFileClient: %v", err)
	}

	if err := client.ReplaceRows(context.Background(), "accounts", [][]any{
		{"name", "type"},
		{"Checking", "Asset"},
	}); err != nil {
		t.Fatalf("replace accounts: %v", err)
	}
	if err := client.ReplaceRows(context.Background(), "transactions", [][]any{
		{"date", "payee"},
		{"2026-05-30", "Smoke Test"},
	}); err != nil {
		t.Fatalf("replace transactions: %v", err)
	}
	if err := client.ReplaceRows(context.Background(), "accounts", [][]any{
		{"name", "type"},
		{"Savings", "Asset"},
	}); err != nil {
		t.Fatalf("replace accounts again: %v", err)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read local sheet: %v", err)
	}
	var got map[string][][]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal local sheet: %v", err)
	}
	if got["accounts"][1][0] != "Savings" {
		t.Fatalf("accounts row = %v, want Savings", got["accounts"][1])
	}
	if got["transactions"][1][1] != "Smoke Test" {
		t.Fatalf("transactions row = %v, want Smoke Test", got["transactions"][1])
	}
}

func TestNewFileClientRequiresPath(t *testing.T) {
	if _, err := NewFileClient(" "); err == nil {
		t.Fatal("NewFileClient should reject blank path")
	}
}
