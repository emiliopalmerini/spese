package core

import (
	"errors"
	"strings"
	"time"
)

const (
	Monthly RepetitionTypes = "monthly"
	Yearly  RepetitionTypes = "yearly"
	Weekly  RepetitionTypes = "weekly"
	Daily   RepetitionTypes = "daily"
)

type (
	RepetitionTypes string

	Date struct {
		time.Time
	}

	Money struct {
		Cents int64
	}

	Expense struct {
		Date        Date
		Description string
		Amount      Money
		Primary     string // Primary category
		Secondary   string // Secondary category
	}

	RecurrentExpenses struct {
		ID          int64 // Database ID for operations
		StartDate   Date
		EndDate     Date
		Every       RepetitionTypes
		Description string
		Amount      Money
		Primary     string // Primary category
		Secondary   string // Secondary category
	}
)

var (
	ErrInvalidDay       = errors.New("invalid day")
	ErrInvalidMonth     = errors.New("invalid month")
	ErrInvalidAmount    = errors.New("invalid amount")
	ErrEmptyDescription = errors.New("empty description")
	ErrEmptyPrimary     = errors.New("empty primary category")
	ErrEmptySecondary   = errors.New("empty secondary category")
)

func (d Date) Validate() error {
	if d.IsZero() {
		return errors.New("date cannot be zero")
	}
	// Check basic ranges
	_, month, day := d.Date()
	if day < 1 || day > 31 {
		return ErrInvalidDay
	}
	if month < 1 || month > 12 {
		return ErrInvalidMonth
	}
	return nil
}

// Day returns the day of the month
func (d Date) Day() int {
	return d.Time.Day()
}

// Month returns the month
func (d Date) Month() int {
	return int(d.Time.Month())
}

// Year returns the year
func (d Date) Year() int {
	return d.Time.Year()
}

// NewDate creates a new Date from year, month, day
func NewDate(year, month, day int) Date {
	return Date{Time: time.Date(year, time.Month(month), day, 0, 0, 0, 0, time.UTC)}
}

// IsEmpty returns true if the date is zero (for backward compatibility with optional dates)
func (d Date) IsEmpty() bool {
	return d.IsZero()
}

func (m Money) Validate() error {
	if m.Cents <= 0 {
		return ErrInvalidAmount
	}
	return nil
}

func (e Expense) Validate() error {
	if err := e.Date.Validate(); err != nil {
		return err
	}
	if len(strings.TrimSpace(e.Description)) == 0 {
		return ErrEmptyDescription
	}
	if len(e.Description) > 200 {
		return errors.New("description too long (max 200 characters)")
	}
	if err := e.Amount.Validate(); err != nil {
		return err
	}
	if strings.TrimSpace(e.Primary) == "" {
		return ErrEmptyPrimary
	}
	if strings.TrimSpace(e.Secondary) == "" {
		return ErrEmptySecondary
	}
	return nil
}

func (re RecurrentExpenses) Validate() error {
	// Validate start date
	if err := re.StartDate.Validate(); err != nil {
		return errors.New("invalid start date: " + err.Error())
	}

	// Validate end date if provided
	if !re.EndDate.IsZero() {
		if err := re.EndDate.Validate(); err != nil {
			return errors.New("invalid end date: " + err.Error())
		}

		// Ensure end date is after start date
		if !re.EndDate.After(re.StartDate.Time) && !re.EndDate.Equal(re.StartDate.Time) {
			return errors.New("end date must be after start date")
		}
	}

	// Validate repetition type
	switch re.Every {
	case Daily, Weekly, Monthly, Yearly:
		// Valid repetition types
	default:
		return errors.New("invalid repetition type")
	}

	// Validate description
	if len(strings.TrimSpace(re.Description)) == 0 {
		return ErrEmptyDescription
	}
	if len(re.Description) > 200 {
		return errors.New("description too long (max 200 characters)")
	}

	// Validate amount
	if err := re.Amount.Validate(); err != nil {
		return err
	}

	// Validate categories
	if strings.TrimSpace(re.Primary) == "" {
		return ErrEmptyPrimary
	}
	if strings.TrimSpace(re.Secondary) == "" {
		return ErrEmptySecondary
	}

	return nil
}
