package render

import "testing"

func TestFmtPctUsesItalianDecimalSeparator(t *testing.T) {
	if got := fmtPct(0.125); got != "12,5%" {
		t.Fatalf("fmtPct(0.125) = %q, want 12,5%%", got)
	}
}
