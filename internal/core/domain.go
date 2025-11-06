// Package core provides the domain model and business logic for the expense tracking application.
//
// It defines the core entities like Expense and RecurrentExpenses, along with their
// validation rules and domain-specific types like Date and Money. The package
// follows domain-driven design principles and provides type-safe representations
// of business concepts.
package core

import (
	"errors"
	"strings"
	"time"
)

// RepetitionTypes constants define the supported frequencies for recurrent expenses.
const (
	Monthly RepetitionTypes = "monthly" // Monthly recurrence
	Yearly  RepetitionTypes = "yearly"  // Yearly recurrence
	Weekly  RepetitionTypes = "weekly"  // Weekly recurrence
	Daily   RepetitionTypes = "daily"   // Daily recurrence
)

// RepetitionTypes represents the frequency type for recurrent expenses.
// It is a string type that can be one of the predefined constants.
type RepetitionTypes string

// Date wraps time.Time to provide domain-specific date handling.
// It provides methods for day, month, and year access while maintaining
// compatibility with Go's standard time package.
type Date struct {
	time.Time
}

// Money represents a monetary amount stored in cents to avoid floating-point precision issues.
// All monetary calculations and storage use cents as the base unit.
type Money struct {
	Cents int64
}

// Expense represents a single expense entry in the system.
// It contains all the necessary information for tracking an individual expense,
// including date, description, amount, and categorization.
type Expense struct {
	Date        Date   // Date when the expense occurred
	Description string // Human-readable description of the expense
	Amount      Money  // Monetary amount in cents
	Primary     string // Primary category (e.g., "Food", "Transport")
	Secondary   string // Secondary category (e.g., "Supermarket", "Public")
}

// RecurrentExpenses represents a recurring expense configuration.
// It defines expenses that occur regularly at specified intervals,
// with optional start and end dates for the recurrence period.
type RecurrentExpenses struct {
	ID          int64          // Database ID for operations
	StartDate   Date           // Date when the recurrence starts
	EndDate     Date           // Optional date when the recurrence ends (zero if indefinite)
	Every       RepetitionTypes // Frequency of recurrence
	Description string         // Human-readable description
	Amount      Money          // Monetary amount in cents per occurrence
	Primary     string         // Primary category
	Secondary   string         // Secondary category
}

// Domain validation errors.
var (
	ErrInvalidDay       = errors.New("invalid day")           // Day value is outside valid range (1-31)
	ErrInvalidMonth     = errors.New("invalid month")         // Month value is outside valid range (1-12)
	ErrInvalidAmount    = errors.New("invalid amount")        // Amount is zero or negative
	ErrEmptyDescription = errors.New("empty description")     // Description field is empty or whitespace-only
	ErrEmptyPrimary     = errors.New("empty primary category") // Primary category is empty
	ErrEmptySecondary   = errors.New("empty secondary category") // Secondary category is empty
)

// Validate checks if the Date represents a valid date.
// It ensures the date is not zero and has valid day/month ranges.
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

// Day returns the day of the month (1-31).
func (d Date) Day() int {
	return d.Time.Day()
}

// Month returns the month as an integer (1-12).
// January is 1, February is 2, etc.
func (d Date) Month() int {
	return int(d.Time.Month())
}

// Year returns the year.
func (d Date) Year() int {
	return d.Time.Year()
}

// NewDate creates a new Date from year, month, and day components.
// The month should be 1-12 (January=1), and day should be valid for the given month.
// The time is set to 00:00:00 UTC.
func NewDate(year, month, day int) Date {
	return Date{Time: time.Date(year, time.Month(month), day, 0, 0, 0, 0, time.UTC)}
}

// IsEmpty returns true if the date is zero (for backward compatibility with optional dates).
// This is equivalent to IsZero() but provides a more descriptive name for optional date fields.
func (d Date) IsEmpty() bool {
	return d.IsZero()
}

// Validate checks if the Money amount is valid.
// It ensures the amount is positive (greater than zero cents).
func (m Money) Validate() error {
	if m.Cents <= 0 {
		return ErrInvalidAmount
	}
	return nil
}

// Validate performs comprehensive validation of an Expense.
// It checks that the date is valid, description is non-empty and not too long,
// amount is positive, and both category fields are non-empty.
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

// Validate performs comprehensive validation of a RecurrentExpenses configuration.
// It checks start date validity, end date validity (if provided), ensures end date
// is after start date, validates repetition type, and checks all other required fields.
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
