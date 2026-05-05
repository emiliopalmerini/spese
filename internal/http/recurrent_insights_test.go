package http

import (
	"testing"
	"time"

	"spese/internal/core"
)

func mkRE(id int64, every core.RepetitionTypes, startY, startM, startD int, cents int64, desc, p, sec string) core.RecurrentExpenses {
	return core.RecurrentExpenses{
		ID:          id,
		StartDate:   core.NewDate(startY, startM, startD),
		Every:       every,
		Description: desc,
		Amount:      core.Money{Cents: cents},
		Primary:     p,
		Secondary:   sec,
	}
}

func TestNextOccurrence(t *testing.T) {
	t.Run("daily returns today", func(t *testing.T) {
		re := mkRE(1, core.Daily, 2025, 1, 1, 500, "x", "p", "s")
		today := time.Date(2026, 5, 5, 0, 0, 0, 0, time.UTC)
		got, ok := nextOccurrence(re, today)
		if !ok || !got.Equal(today) {
			t.Fatalf("want %v ok=true, got %v ok=%v", today, got, ok)
		}
	})

	t.Run("weekly aligns to start weekday", func(t *testing.T) {
		// Start Monday 2025-01-06. Today Tue 2026-05-05 → next Monday 2026-05-11.
		re := mkRE(1, core.Weekly, 2025, 1, 6, 500, "x", "p", "s")
		today := time.Date(2026, 5, 5, 0, 0, 0, 0, time.UTC)
		want := time.Date(2026, 5, 11, 0, 0, 0, 0, time.UTC)
		got, ok := nextOccurrence(re, today)
		if !ok || !got.Equal(want) {
			t.Fatalf("want %v ok=true, got %v ok=%v", want, got, ok)
		}
	})

	t.Run("weekly today matches weekday", func(t *testing.T) {
		// Start Tue 2025-01-07. Today Tue 2026-05-05.
		re := mkRE(1, core.Weekly, 2025, 1, 7, 500, "x", "p", "s")
		today := time.Date(2026, 5, 5, 0, 0, 0, 0, time.UTC)
		got, ok := nextOccurrence(re, today)
		if !ok || !got.Equal(today) {
			t.Fatalf("want %v ok=true, got %v ok=%v", today, got, ok)
		}
	})

	t.Run("monthly day 31 caps to 30 in April", func(t *testing.T) {
		re := mkRE(1, core.Monthly, 2025, 1, 31, 500, "x", "p", "s")
		today := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
		want := time.Date(2026, 4, 30, 0, 0, 0, 0, time.UTC)
		got, ok := nextOccurrence(re, today)
		if !ok || !got.Equal(want) {
			t.Fatalf("want %v ok=true, got %v ok=%v", want, got, ok)
		}
	})

	t.Run("monthly rolls to next month when day passed", func(t *testing.T) {
		re := mkRE(1, core.Monthly, 2025, 1, 5, 500, "x", "p", "s")
		today := time.Date(2026, 5, 10, 0, 0, 0, 0, time.UTC)
		want := time.Date(2026, 6, 5, 0, 0, 0, 0, time.UTC)
		got, ok := nextOccurrence(re, today)
		if !ok || !got.Equal(want) {
			t.Fatalf("want %v ok=true, got %v ok=%v", want, got, ok)
		}
	})

	t.Run("yearly Feb 29 falls back to Feb 28 non-leap", func(t *testing.T) {
		re := mkRE(1, core.Yearly, 2024, 2, 29, 500, "x", "p", "s")
		today := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
		want := time.Date(2025, 2, 28, 0, 0, 0, 0, time.UTC)
		got, ok := nextOccurrence(re, today)
		if !ok || !got.Equal(want) {
			t.Fatalf("want %v ok=true, got %v ok=%v", want, got, ok)
		}
	})

	t.Run("expired EndDate returns false", func(t *testing.T) {
		re := mkRE(1, core.Monthly, 2025, 1, 5, 500, "x", "p", "s")
		re.EndDate = core.NewDate(2026, 1, 1)
		today := time.Date(2026, 5, 5, 0, 0, 0, 0, time.UTC)
		_, ok := nextOccurrence(re, today)
		if ok {
			t.Fatalf("want ok=false on expired EndDate")
		}
	})

	t.Run("EndDate before next occurrence returns false", func(t *testing.T) {
		re := mkRE(1, core.Monthly, 2025, 1, 20, 500, "x", "p", "s")
		re.EndDate = core.NewDate(2026, 5, 15)
		today := time.Date(2026, 5, 5, 0, 0, 0, 0, time.UTC)
		// Next would be 2026-05-20 which is after EndDate 2026-05-15.
		_, ok := nextOccurrence(re, today)
		if ok {
			t.Fatalf("want ok=false when next is past EndDate")
		}
	})
}

func TestBuildInsights_Empty(t *testing.T) {
	now := time.Date(2026, 5, 5, 0, 0, 0, 0, time.UTC)
	vm := buildInsights(nil, now)
	if vm.HasData {
		t.Fatalf("want HasData=false")
	}
	if vm.ActiveCount != 0 {
		t.Fatalf("want ActiveCount=0, got %d", vm.ActiveCount)
	}
	if len(vm.Frequencies) != 4 {
		t.Fatalf("want 4 frequency cells, got %d", len(vm.Frequencies))
	}
	for _, f := range vm.Frequencies {
		if f.Count != 0 || f.MonthlyFmt != "—" {
			t.Fatalf("want zero/em-dash, got %+v", f)
		}
	}
}

func TestBuildInsights_Aggregates(t *testing.T) {
	now := time.Date(2026, 5, 5, 0, 0, 0, 0, time.UTC)
	expenses := []core.RecurrentExpenses{
		mkRE(1, core.Monthly, 2025, 1, 10, 1000_00, "Rent", "Casa", "Affitto"),
		mkRE(2, core.Monthly, 2025, 1, 20, 50_00, "Spotify", "Svago", "Streaming"),
		mkRE(3, core.Yearly, 2025, 6, 1, 12000_00, "Insurance", "Casa", "Assicurazione"),
		mkRE(4, core.Weekly, 2025, 1, 6, 25_00, "Cleaning", "Casa", "Pulizie"),
		mkRE(5, core.Daily, 2025, 1, 1, 5_00, "Coffee", "Svago", "Bar"),
	}

	vm := buildInsights(expenses, now)

	if !vm.HasData {
		t.Fatalf("want HasData=true")
	}
	if vm.ActiveCount != 5 {
		t.Fatalf("want ActiveCount=5, got %d", vm.ActiveCount)
	}

	// monthly totals: 1000 + 50 + (12000/12=1000) + (25*4=100) + (5*30=150) = 2300 €
	wantTotal := int64(1000_00 + 50_00 + 1000_00 + 100_00 + 150_00)
	if vm.MonthlyTotalFmt != formatEuros(wantTotal) {
		t.Fatalf("monthly total mismatch: want %s got %s", formatEuros(wantTotal), vm.MonthlyTotalFmt)
	}

	// 4 frequency cells, all with count > 0
	if len(vm.Frequencies) != 4 {
		t.Fatalf("want 4 freq cells, got %d", len(vm.Frequencies))
	}
	gotCount := map[string]int{}
	for _, f := range vm.Frequencies {
		gotCount[f.Key] = f.Count
	}
	if gotCount["daily"] != 1 || gotCount["weekly"] != 1 || gotCount["monthly"] != 2 || gotCount["yearly"] != 1 {
		t.Fatalf("freq counts wrong: %+v", gotCount)
	}

	// Top-5 ordering: Rent and Insurance both 1000€ tie at top, but stable sort
	// keeps input order; Rent (id=1) before Insurance (id=3).
	if len(vm.Top5) != 5 {
		t.Fatalf("want 5 top rows, got %d", len(vm.Top5))
	}
	if vm.Top5[0].Name != "Rent" {
		t.Fatalf("want Rent at top, got %s", vm.Top5[0].Name)
	}
	if vm.Top5[0].WidthPct != 100 {
		t.Fatalf("want top width 100, got %d", vm.Top5[0].WidthPct)
	}

	// Categories: Casa = 1000+1000+100 = 2100, Svago = 50 + 150 = 200
	if len(vm.Categories) != 2 {
		t.Fatalf("want 2 category groups, got %d", len(vm.Categories))
	}
	if vm.Categories[0].Name != "Casa" {
		t.Fatalf("want Casa first (largest), got %s", vm.Categories[0].Name)
	}
	if len(vm.Categories[0].Children) != 3 {
		t.Fatalf("want 3 secondaries under Casa, got %d", len(vm.Categories[0].Children))
	}
}

func TestBuildInsights_ExcludesExpired(t *testing.T) {
	now := time.Date(2026, 5, 5, 0, 0, 0, 0, time.UTC)
	live := mkRE(1, core.Monthly, 2025, 1, 10, 100_00, "Live", "p", "s")
	dead := mkRE(2, core.Monthly, 2025, 1, 10, 999_00, "Dead", "p", "s")
	dead.EndDate = core.NewDate(2026, 1, 1)
	vm := buildInsights([]core.RecurrentExpenses{live, dead}, now)
	if vm.ActiveCount != 1 {
		t.Fatalf("want ActiveCount=1, got %d", vm.ActiveCount)
	}
	if vm.MonthlyTotalFmt != formatEuros(100_00) {
		t.Fatalf("want only live total, got %s", vm.MonthlyTotalFmt)
	}
}

func TestBuildInsights_UpcomingWindowAndOrder(t *testing.T) {
	now := time.Date(2026, 5, 5, 0, 0, 0, 0, time.UTC)
	// monthly on day 10 → next 2026-05-10
	a := mkRE(1, core.Monthly, 2025, 1, 10, 100_00, "A", "p", "s")
	// monthly on day 7 → next 2026-05-07
	b := mkRE(2, core.Monthly, 2025, 1, 7, 100_00, "B", "p", "s")
	// yearly on 2025-12-01 → next 2026-12-01 → outside 30-day window
	c := mkRE(3, core.Yearly, 2025, 12, 1, 12000_00, "C", "p", "s")

	vm := buildInsights([]core.RecurrentExpenses{a, b, c}, now)

	if len(vm.Upcoming) != 2 {
		t.Fatalf("want 2 upcoming rows, got %d (%+v)", len(vm.Upcoming), vm.Upcoming)
	}
	if vm.Upcoming[0].Name != "B" {
		t.Fatalf("want B before A by date, got %s", vm.Upcoming[0].Name)
	}
	if vm.Upcoming[0].DateShort != "07 Mag" {
		t.Fatalf("want '07 Mag', got %q", vm.Upcoming[0].DateShort)
	}
}
