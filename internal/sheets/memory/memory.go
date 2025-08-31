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

// Interface conformance
var _ interface {
	Append(context.Context, core.Expense) (string, error)
	List(context.Context) ([]string, []string, error)
	ReadMonthOverview(context.Context, int, int) (core.MonthOverview, error)
} = (*Store)(nil)

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

// ReadMonthOverview aggregates stored items for the given month.
// Year is currently ignored in memory backend.
func (s *Store) ReadMonthOverview(_ context.Context, year int, month int) (core.MonthOverview, error) { //nolint:revive // year unused in memory
	_ = year
	s.mu.Lock()
	defer s.mu.Unlock()
	if month < 1 || month > 12 {
		month = 1
	}
	byCat := map[string]int64{}
	var total int64
	for _, e := range s.items {
		if e.Date.Month != month {
			continue
		}
		byCat[e.Primary] += e.Amount.Cents
		total += e.Amount.Cents
	}
	// Build deterministic order preserving insertion by iterating cats list first
	var list []core.CategoryAmount
	seen := map[string]bool{}
	for _, c := range s.cats {
		if amt, ok := byCat[c]; ok {
			list = append(list, core.CategoryAmount{Name: c, Amount: core.Money{Cents: amt}})
			seen[c] = true
		}
	}
	for c, amt := range byCat {
		if seen[c] {
			continue
		}
		list = append(list, core.CategoryAmount{Name: c, Amount: core.Money{Cents: amt}})
	}
	return core.MonthOverview{
		Year:       year,
		Month:      month,
		Total:      core.Money{Cents: total},
		ByCategory: list,
	}, nil
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
