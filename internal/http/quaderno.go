package http

import (
	"fmt"
	"strconv"
	"time"
)

// Italian month names (long + short).
var italianMonthsLong = [...]string{
	"Gennaio", "Febbraio", "Marzo", "Aprile", "Maggio", "Giugno",
	"Luglio", "Agosto", "Settembre", "Ottobre", "Novembre", "Dicembre",
}
var italianMonthsShort = [...]string{
	"Gen", "Feb", "Mar", "Apr", "Mag", "Giu",
	"Lug", "Ago", "Set", "Ott", "Nov", "Dic",
}

// italianMonthLong returns the Italian long month name for month in 1..12.
// Out-of-range input yields an empty string.
func italianMonthLong(month int) string {
	if month < 1 || month > 12 {
		return ""
	}
	return italianMonthsLong[month-1]
}

// italianMonthShort returns the Italian short month name for month in 1..12.
// Out-of-range input yields an empty string.
func italianMonthShort(month int) string {
	if month < 1 || month > 12 {
		return ""
	}
	return italianMonthsShort[month-1]
}

// romanNumeral returns the Roman numeral for n in 1..12. Out-of-range yields "".
func romanNumeral(n int) string {
	romans := [...]string{
		"I", "II", "III", "IV", "V", "VI", "VII", "VIII", "IX", "X", "XI", "XII",
	}
	if n < 1 || n > 12 {
		return ""
	}
	return romans[n-1]
}

// signedDeltaPct computes a rounded integer percent delta of curr vs prev.
// Returns 0 when prev is zero (avoid div-by-zero); positive means curr > prev.
// Rounding uses round-half-away-from-zero.
func signedDeltaPct(curr, prev int64) int {
	if prev == 0 {
		return 0
	}
	diff := float64(curr - prev)
	pct := (diff / float64(absInt64(prev))) * 100.0
	if pct >= 0 {
		return int(pct + 0.5)
	}
	return int(pct - 0.5)
}

// dailyRunRate returns total / max(dayOfMonth, 1).
func dailyRunRate(totalCents int64, dayOfMonth int) int64 {
	if dayOfMonth < 1 {
		dayOfMonth = 1
	}
	return totalCents / int64(dayOfMonth)
}

// prevYearMonth returns the year and month preceding the given (year, month).
func prevYearMonth(year, month int) (int, int) {
	pm := month - 1
	py := year
	if pm < 1 {
		pm = 12
		py--
	}
	return py, pm
}

func absInt64(n int64) int64 {
	if n < 0 {
		return -n
	}
	return n
}

// formatEurosInt formats cents as a thousands-grouped Italian integer (no decimals,
// no currency glyph). Used for the integer portion of the Quaderno hero number.
func formatEurosInt(cents int64) string {
	euros := cents / 100
	neg := euros < 0
	if neg {
		euros = -euros
	}
	s := strconv.FormatInt(euros, 10)
	// Insert dot every 3 digits from the right.
	out := make([]byte, 0, len(s)+len(s)/3)
	for i, c := range []byte(s) {
		if i > 0 && (len(s)-i)%3 == 0 {
			out = append(out, '.')
		}
		out = append(out, c)
	}
	if neg {
		return "-" + string(out)
	}
	return string(out)
}

// formatEurosDec returns the two-digit decimal portion of cents (e.g. "07", "42").
func formatEurosDec(cents int64) string {
	if cents < 0 {
		cents = -cents
	}
	return fmt.Sprintf("%02d", cents%100)
}

// asOfDay returns the day-of-month of t (1..31).
func asOfDay(t time.Time) int { return t.Day() }

// intAbs returns |n|.
func intAbs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}
