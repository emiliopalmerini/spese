package memory

import (
	"bufio"
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
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
	ListExpenses(context.Context, int, int) ([]core.Expense, error)
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
	s := New(cats, subs)
	// Optionally seed expenses from CSV if present.
	seedCSV := filepath.Join(base, "seed_expenses.csv")
	if _, err := os.Stat(seedCSV); err == nil {
		if n, err := s.loadExpensesCSV(seedCSV); err == nil {
			_ = n // loaded successfully
		}
	}
	return s
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

// ListExpenses returns stored expenses for the given month (year ignored).
func (s *Store) ListExpenses(_ context.Context, year int, month int) ([]core.Expense, error) { //nolint:revive // year unused
	_ = year
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []core.Expense
	for _, e := range s.items {
		if e.Date.Month == month {
			out = append(out, e)
		}
	}
	return out, nil
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

// loadExpensesCSV reads a simple CSV with columns:
// Month,Day,Description,Amount,Primary,Secondary
// Amount is a decimal string in euros.
func (s *Store) loadExpensesCSV(path string) (int, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	r := csv.NewReader(bufio.NewReader(f))
	r.TrimLeadingSpace = true
	count := 0
	for {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return count, err
		}
		if len(rec) == 0 {
			continue
		}
		// Skip header or commented lines
		if strings.HasPrefix(strings.ToLower(strings.TrimSpace(rec[0])), "month") || strings.HasPrefix(strings.TrimSpace(rec[0]), "#") {
			continue
		}
		// Ensure at least 6 columns
		for len(rec) < 6 {
			rec = append(rec, "")
		}
		month, _ := strconv.Atoi(strings.TrimSpace(rec[0]))
		day, _ := strconv.Atoi(strings.TrimSpace(rec[1]))
		desc := strings.TrimSpace(rec[2])
		amtStr := strings.TrimSpace(rec[3])
		primary := strings.TrimSpace(rec[4])
		secondary := strings.TrimSpace(rec[5])
		cents, err := core.ParseDecimalToCents(amtStr)
		if err != nil {
			continue // skip invalid amounts
		}
		e := core.Expense{
			Date:        core.DateParts{Day: day, Month: month},
			Description: desc,
			Amount:      core.Money{Cents: cents},
			Primary:     primary,
			Secondary:   secondary,
		}
		if err := e.Validate(); err != nil {
			continue
		}
		s.items = append(s.items, e)
		count++
	}
	return count, nil
}
