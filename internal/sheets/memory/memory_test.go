package memory

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"spese/internal/core"
)

func TestMemoryStoreAppendAndList(t *testing.T) {
	s := New([]string{"A", "B", "A"}, []string{"X", "Y", "X"})
	cats, subs, err := s.List(context.Background())
	if err != nil || len(cats) != 2 || len(subs) != 2 {
		t.Fatalf("unexpected list: cats=%v subs=%v err=%v", cats, subs, err)
	}

	ref, err := s.Append(context.Background(), core.Expense{
		Date:        core.DateParts{Day: 1, Month: 1},
		Description: "t",
		Amount:      core.Money{Cents: 123},
		Category:    "A",
		Subcategory: "X",
	})
	if err != nil || ref != "mem:1" {
		t.Fatalf("unexpected append: ref=%q err=%v", ref, err)
	}
}

func TestNewFromFilesSeedsAndDedupe(t *testing.T) {
	dir := t.TempDir()
	// No files -> defaults
	s := NewFromFiles(dir)
	cats, subs, _ := s.List(context.Background())
	if len(cats) == 0 || len(subs) == 0 {
		t.Fatalf("expected defaults when files missing")
	}

	// Create files with duplicates, blanks and comments
	mustWrite := func(name, content string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	mustWrite("seed_categories.txt", "# header\nA\nB\nA\n\n")
	mustWrite("seed_subcategories.txt", "# header\nX\nX\nY\n\n")

	s = NewFromFiles(dir)
	cats, subs, _ = s.List(context.Background())
	if len(cats) != 2 || cats[0] != "A" || cats[1] != "B" {
		t.Fatalf("unexpected cats: %v", cats)
	}
	if len(subs) != 2 || subs[0] != "X" || subs[1] != "Y" {
		t.Fatalf("unexpected subs: %v", subs)
	}
}
