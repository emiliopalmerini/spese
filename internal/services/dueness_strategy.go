// Package services provides business logic and orchestration services.
//
// This file implements the Strategy Pattern for recurring expense dueness checking.
// Each frequency type (daily, weekly, monthly, yearly) has its own strategy
// that encapsulates the logic for determining if an expense is due.

package services

import (
	"fmt"
	"spese/internal/core"
	"time"
)

// DuenessChecker is the strategy interface for checking if a recurring expense is due.
// Each implementation encapsulates the algorithm for a specific frequency type.
type DuenessChecker interface {
	// IsDue returns true if the recurring expense should be processed based on
	// the last execution time and the current time.
	IsDue(lastExecution, now time.Time, startDate core.Date) bool
}

// DailyChecker implements DuenessChecker for daily recurring expenses.
type DailyChecker struct{}

// IsDue returns true if last execution was before today.
func (DailyChecker) IsDue(lastExecution, now time.Time, _ core.Date) bool {
	if lastExecution.IsZero() {
		return true
	}
	lastDate := lastExecution.Format("2006-01-02")
	nowDate := now.Format("2006-01-02")
	return lastDate != nowDate
}

// WeeklyChecker implements DuenessChecker for weekly recurring expenses.
type WeeklyChecker struct{}

// IsDue returns true if 7 or more days have passed since last execution.
func (WeeklyChecker) IsDue(lastExecution, now time.Time, _ core.Date) bool {
	if lastExecution.IsZero() {
		return true
	}
	daysSince := now.Sub(lastExecution).Hours() / 24
	return daysSince >= 7
}

// MonthlyChecker implements DuenessChecker for monthly recurring expenses.
type MonthlyChecker struct{}

// IsDue returns true if we're in a new month and have reached the target day.
func (MonthlyChecker) IsDue(lastExecution, now time.Time, startDate core.Date) bool {
	if lastExecution.IsZero() {
		return true
	}

	// Already processed this month?
	if lastExecution.Year() == now.Year() && lastExecution.Month() == now.Month() {
		return false
	}

	// Check if we've reached the target day of the month
	targetDay := startDate.Day()
	targetDayThisMonth := targetDay
	lastDayOfMonth := time.Date(now.Year(), now.Month()+1, 0, 0, 0, 0, 0, time.UTC).Day()
	if targetDay > lastDayOfMonth {
		targetDayThisMonth = lastDayOfMonth
	}

	return now.Day() >= targetDayThisMonth
}

// YearlyChecker implements DuenessChecker for yearly recurring expenses.
type YearlyChecker struct{}

// IsDue returns true if we're in a new year and have reached the target month and day.
func (YearlyChecker) IsDue(lastExecution, now time.Time, startDate core.Date) bool {
	if lastExecution.IsZero() {
		return true
	}

	// Already processed this year?
	if lastExecution.Year() == now.Year() {
		return false
	}

	targetMonth := startDate.Month()
	targetDay := startDate.Day()

	// Check if we've reached the target month and day
	if int(now.Month()) < targetMonth {
		return false
	}

	if int(now.Month()) == targetMonth {
		lastDayOfMonth := time.Date(now.Year(), now.Month()+1, 0, 0, 0, 0, 0, time.UTC).Day()
		targetDayThisMonth := targetDay
		if targetDay > lastDayOfMonth {
			targetDayThisMonth = lastDayOfMonth
		}
		return now.Day() >= targetDayThisMonth
	}

	// We're past the target month
	return true
}

// duenessStrategies maps repetition types to their corresponding checkers.
// This registry enables O(1) lookup and easy extension for new frequency types.
var duenessStrategies = map[core.RepetitionTypes]DuenessChecker{
	core.Daily:   DailyChecker{},
	core.Weekly:  WeeklyChecker{},
	core.Monthly: MonthlyChecker{},
	core.Yearly:  YearlyChecker{},
}

// GetDuenessChecker returns the appropriate dueness checker for a repetition type.
// Returns an error if the repetition type is not supported.
func GetDuenessChecker(frequency core.RepetitionTypes) (DuenessChecker, error) {
	checker, ok := duenessStrategies[frequency]
	if !ok {
		return nil, fmt.Errorf("unknown repetition type: %s", frequency)
	}
	return checker, nil
}

// RegisterDuenessChecker allows registering custom dueness checkers for new frequency types.
// This supports the Open/Closed principle by allowing extension without modification.
func RegisterDuenessChecker(frequency core.RepetitionTypes, checker DuenessChecker) {
	duenessStrategies[frequency] = checker
}
