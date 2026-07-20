package dashboard

import "testing"

func TestParseDashboardPeriodUsesRequestedMonth(t *testing.T) {
	period, err := parseDashboardPeriod("2026-04")
	if err != nil {
		t.Fatal(err)
	}
	if period.Month() != "2026-04" {
		t.Fatalf("period = %q, want 2026-04", period.Month())
	}
	if periodURL(period) != "/?month=2026-04" {
		t.Fatalf("period URL = %q", periodURL(period))
	}
}

func TestParseDashboardPeriodRejectsInvalidMonth(t *testing.T) {
	if _, err := parseDashboardPeriod("not-a-month"); err == nil {
		t.Fatal("expected invalid month error")
	}
}
