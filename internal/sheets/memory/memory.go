package memory

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"spese/internal/core"
)

type Store struct {
	mu    sync.Mutex
	cats  []string
	subs  []string
	items []core.Expense
}

func New(cats, subs []string) *Store {
	return &Store{cats: dedupeSorted(cats), subs: dedupeSorted(subs)}
}

func NewFromFiles(base string) *Store {
	cats := readLines(filepath.Join(base, "seed_categories.txt"))
	subs := readLines(filepath.Join(base, "seed_subcategories.txt"))
	if len(cats) == 0 {
		cats = []string{"Casa", "Cibo", "Trasporti"}
	}
	if len(subs) == 0 {
		subs = []string{"Generale", "Supermercato", "Ristorante"}
	}
	return New(cats, subs)
}

// Append stores the expense and returns a synthetic row reference.
func (s *Store) Append(_ context.Context, e core.Expense) (string, error) {
	if err := e.Validate(); err != nil {
		return "", err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.items = append(s.items, e)
	return fmt.Sprintf("mem:%d", len(s.items)), nil
}

// List returns categories and subcategories.
func (s *Store) List(_ context.Context) ([]string, []string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	cats := append([]string(nil), s.cats...)
	subs := append([]string(nil), s.subs...)
	return cats, subs, nil
}

func readLines(path string) []string {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	var out []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		out = append(out, line)
	}
	return dedupeSorted(out)
}

func dedupeSorted(in []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(in))
	for _, v := range in {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	// Preserve input order; sorting is optional and can be added later.
	return out
}
