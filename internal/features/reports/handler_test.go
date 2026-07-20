package reports

import (
	"net/url"
	"testing"
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
