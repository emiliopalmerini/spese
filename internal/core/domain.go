package core

import "errors"

type (
	DateParts struct {
		Day   int
		Month int
	}

	Money struct {
		Cents int64
	}

	Expense struct {
		Date        DateParts
		Description string
		Amount      Money
		Category    string
		Subcategory string
	}
)

var (
	ErrInvalidDay       = errors.New("invalid day")
	ErrInvalidMonth     = errors.New("invalid month")
	ErrInvalidAmount    = errors.New("invalid amount")
	ErrEmptyDescription = errors.New("empty description")
	ErrEmptyCategory    = errors.New("empty category")
	ErrEmptySubcategory = errors.New("empty subcategory")
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
	if len(e.Description) == 0 {
		return ErrEmptyDescription
	}
	if err := e.Amount.Validate(); err != nil {
		return err
	}
	if e.Category == "" {
		return ErrEmptyCategory
	}
	if e.Subcategory == "" {
		return ErrEmptySubcategory
	}
	return nil
}
