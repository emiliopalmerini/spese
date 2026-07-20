package render

import (
	"testing"

	"spese/internal/kernel"
)

func TestFmtPctUsesItalianDecimalSeparator(t *testing.T) {
	if got := fmtPct(0.125); got != "12,5%" {
		t.Fatalf("fmtPct(0.125) = %q, want 12,5%%", got)
	}
}

func TestFmtDateITUsesReadableItalianDate(t *testing.T) {
	date, err := kernel.ParseDate("2026-05-15")
	if err != nil {
		t.Fatal(err)
	}
	if got := fmtDateIT(date); got != "15 mag 2026" {
		t.Fatalf("fmtDateIT() = %q, want 15 mag 2026", got)
	}
}
