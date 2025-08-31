package core

import (
	"errors"
	"strings"
)

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
		Primary     string // Primary category
		Secondary   string // Secondary category  
	}
)

var (
	ErrInvalidDay       = errors.New("invalid day")
	ErrInvalidMonth     = errors.New("invalid month")
	ErrInvalidAmount    = errors.New("invalid amount")
	ErrEmptyDescription = errors.New("empty description")
	ErrEmptyPrimary   = errors.New("empty primary category")
	ErrEmptySecondary = errors.New("empty secondary category")
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
