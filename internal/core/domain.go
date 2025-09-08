package core

import (
	"errors"
	"strings"
)

const (
	Monthly RepetitionTypes = "monthly"
	Yearly  RepetitionTypes = "yearly"
	Weekly  RepetitionTypes = "weekly"
	Daily   RepetitionTypes = "daily"
)

type (
	RepetitionTypes string

	DateParts struct {
		Day   int
		Month int
		Year  int
	}

	Money struct {
		Cents int64
	}

	Expense struct {
		Date        DateParts
		Description string
		Amount      Money
		Primary     string // Primary category
		Secondary   string // Secondary category
	}

	RecurrentExpenses struct {
		StartDate   DateParts
		EndDate     DateParts
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

func (d DateParts) Validate() error {
	if d.Day < 1 || d.Day > 31 {
		return ErrInvalidDay
	}
	if d.Month < 1 || d.Month > 12 {
		return ErrInvalidMonth
	}
	return nil
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
	if re.EndDate.Year > 0 || re.EndDate.Month > 0 || re.EndDate.Day > 0 {
		if err := re.EndDate.Validate(); err != nil {
			return errors.New("invalid end date: " + err.Error())
		}
		
		// Ensure end date is after start date
		startTime := re.StartDate.Year*10000 + re.StartDate.Month*100 + re.StartDate.Day
		endTime := re.EndDate.Year*10000 + re.EndDate.Month*100 + re.EndDate.Day
		if endTime < startTime {
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
