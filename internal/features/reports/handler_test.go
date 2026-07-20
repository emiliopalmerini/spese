package reports

import (
	"net/url"
	"testing"

	"spese/internal/kernel"
)

func TestParseReportPeriodBuildsInclusiveMonthRange(t *testing.T) {
	period, form, err := parseReportPeriod(url.Values{"from": {"2026-01"}, "to": {"2026-03"}})
	if err != nil {
		t.Fatal(err)
	}
	if period.From.Month() != "2026-01" || period.To.Month() != "2026-04" {
		t.Fatalf("period = %s to %s, want January through March", period.From.Month(), period.To.Month())
	}
	if form.From != "2026-01" || form.To != "2026-03" {
		t.Fatalf("form = %+v", form)
	}
}

func TestFilterIncomeRowsUsesReportPeriod(t *testing.T) {
	period, _, err := parseReportPeriod(url.Values{"from": {"2026-02"}, "to": {"2026-02"}})
	if err != nil {
		t.Fatal(err)
	}
	rows := filterIncomeRows([]IncomeRow{
		{Month: reportDate(t, "2026-01")},
		{Month: reportDate(t, "2026-02")},
		{Month: reportDate(t, "2026-03")},
	}, period)
	if len(rows) != 1 || rows[0].Month.Month() != "2026-02" {
		t.Fatalf("rows = %+v, want only February", rows)
	}
}

func TestResolveReportPeriodSuggestsLatestTwelveAvailableMonths(t *testing.T) {
	period, form := resolveReportPeriod(reportPeriod{}, PeriodFilterView{}, []kernel.Date{
		reportDate(t, "2025-01"),
		reportDate(t, "2026-06"),
	})

	if form.From != "2025-07" || form.To != "2026-06" {
		t.Fatalf("suggested range = %q to %q, want latest twelve months", form.From, form.To)
	}
	if form.Min != "2025-01" || form.Max != "2026-06" {
		t.Fatalf("available range = %q to %q", form.Min, form.Max)
	}
	if period.From.Month() != "2025-07" || period.To.Month() != "2026-07" {
		t.Fatalf("filter period = %q to %q", period.From.Month(), period.To.Month())
	}
}
