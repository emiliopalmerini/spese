package render

import (
	"fmt"
	"html/template"
	"strings"
	"time"

	"spese/internal/kernel"
)

// funcs is the template function map exposed in every page.
var funcs = template.FuncMap{
	"money":        fmtMoney,
	"pct":          fmtPct,
	"isodate":      fmtISODate,
	"dateIT":       fmtDateIT,
	"monthIT":      fmtMonthIT,
	"title":        cases.Title,
	"join":         strings.Join,
	"nonempty":     nonempty,
	"accountType":  accountTypeLabel,
	"accountClass": accountClassLabel,
}

func fmtMoney(m any) string {
	switch v := m.(type) {
	case kernel.Money:
		return v.String() + " €"
	case float64:
		return kernel.Money(int64(v*100)).String() + " €"
	case int64:
		return kernel.Money(v).String() + " €"
	default:
		return ""
	}
}

func fmtPct(v any) string {
	f, ok := v.(float64)
	if !ok {
		return ""
	}
	return strings.Replace(fmt.Sprintf("%.1f%%", f*100), ".", ",", 1)
}

func fmtISODate(v any) string {
	switch d := v.(type) {
	case kernel.Date:
		if d.IsZero() {
			return ""
		}
		return d.ISO()
	case time.Time:
		if d.IsZero() {
			return ""
		}
		return d.Format("2006-01-02")
	}
	return ""
}

func fmtDateIT(v any) string {
	var value time.Time
	switch date := v.(type) {
	case kernel.Date:
		value = date.Time
	case time.Time:
		value = date
	default:
		return ""
	}
	if value.IsZero() {
		return ""
	}
	months := [...]string{"gen", "feb", "mar", "apr", "mag", "giu", "lug", "ago", "set", "ott", "nov", "dic"}
	return fmt.Sprintf("%d %s %d", value.Day(), months[value.Month()-1], value.Year())
}

// fmtMonthIT renders a kernel.Date as "Aprile 2026" (italian month name).
func fmtMonthIT(v any) string {
	var t time.Time
	switch d := v.(type) {
	case kernel.Date:
		if d.IsZero() {
			return ""
		}
		t = d.Time
	case time.Time:
		if d.IsZero() {
			return ""
		}
		t = d
	default:
		return ""
	}
	months := []string{
		"Gennaio", "Febbraio", "Marzo", "Aprile", "Maggio", "Giugno",
		"Luglio", "Agosto", "Settembre", "Ottobre", "Novembre", "Dicembre",
	}
	return fmt.Sprintf("%s %d", months[t.Month()-1], t.Year())
}

func nonempty(s string) bool { return strings.TrimSpace(s) != "" }

func accountTypeLabel(raw any) string {
	value := fmt.Sprint(raw)
	if label := map[string]string{
		"Asset":     "Attività",
		"Liability": "Passività",
	}[value]; label != "" {
		return label
	}
	return value
}

func accountClassLabel(raw any) string {
	value := fmt.Sprint(raw)
	if label := map[string]string{
		"Cash":       "Liquidità",
		"Investment": "Investimenti",
		"Property":   "Immobili",
		"Tax":        "Imposte",
		"Credit":     "Credito",
		"Other":      "Altro",
	}[value]; label != "" {
		return label
	}
	return value
}

// cases avoids importing golang.org/x/text/cases for one helper; we just
// need Italian Title case which is the same as Go's deprecated strings.Title.
var cases = struct {
	Title func(string) string
}{
	Title: func(s string) string {
		if s == "" {
			return s
		}
		runes := []rune(strings.ToLower(s))
		runes[0] = []rune(strings.ToUpper(string(runes[0])))[0]
		return string(runes)
	},
}
