package kernel

import (
	"fmt"
	"time"
)

// Date is a calendar date with no time-of-day component, in the spreadsheet's
// implicit timezone (Europe/Rome). Used for transaction dates and snapshot
// month markers.
type Date struct{ time.Time }

// Today returns the current local date.
func Today() Date {
	now := time.Now()
	return Date{time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local)}
}

// ParseDate accepts "2026-04-15" (ISO) or "2026-04" (month) and returns
// the first day of that month for the latter.
func ParseDate(s string) (Date, error) {
	if len(s) == 7 {
		t, err := time.ParseInLocation("2006-01", s, time.Local)
		if err != nil {
			return Date{}, fmt.Errorf("parse date %q: %w", s, err)
		}
		return Date{t}, nil
	}
	t, err := time.ParseInLocation("2006-01-02", s, time.Local)
	if err != nil {
		return Date{}, fmt.Errorf("parse date %q: %w", s, err)
	}
	return Date{t}, nil
}

// ISO returns the date in yyyy-mm-dd form.
func (d Date) ISO() string { return d.Format("2006-01-02") }

// Month returns the date in yyyy-mm form.
func (d Date) Month() string { return d.Format("2006-01") }

// FirstOfMonth returns the same date snapped back to day 1.
func (d Date) FirstOfMonth() Date {
	t := d.Time
	return Date{time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, t.Location())}
}
