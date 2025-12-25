package services

import (
	"spese/internal/core"
	"testing"
	"time"
)

func TestDailyChecker_IsDue(t *testing.T) {
	checker := DailyChecker{}
	now := time.Date(2024, 1, 15, 12, 0, 0, 0, time.UTC)
	startDate := core.NewDate(2024, 1, 1)

	tests := []struct {
		name          string
		lastExecution time.Time
		want          bool
	}{
		{
			name:          "never executed - is due",
			lastExecution: time.Time{},
			want:          true,
		},
		{
			name:          "executed today - not due",
			lastExecution: time.Date(2024, 1, 15, 8, 0, 0, 0, time.UTC),
			want:          false,
		},
		{
			name:          "executed yesterday - is due",
			lastExecution: time.Date(2024, 1, 14, 12, 0, 0, 0, time.UTC),
			want:          true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := checker.IsDue(tt.lastExecution, now, startDate)
			if got != tt.want {
				t.Errorf("DailyChecker.IsDue() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestWeeklyChecker_IsDue(t *testing.T) {
	checker := WeeklyChecker{}
	now := time.Date(2024, 1, 15, 12, 0, 0, 0, time.UTC)
	startDate := core.NewDate(2024, 1, 1)

	tests := []struct {
		name          string
		lastExecution time.Time
		want          bool
	}{
		{
			name:          "never executed - is due",
			lastExecution: time.Time{},
			want:          true,
		},
		{
			name:          "executed 3 days ago - not due",
			lastExecution: time.Date(2024, 1, 12, 12, 0, 0, 0, time.UTC),
			want:          false,
		},
		{
			name:          "executed 7 days ago - is due",
			lastExecution: time.Date(2024, 1, 8, 12, 0, 0, 0, time.UTC),
			want:          true,
		},
		{
			name:          "executed 10 days ago - is due",
			lastExecution: time.Date(2024, 1, 5, 12, 0, 0, 0, time.UTC),
			want:          true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := checker.IsDue(tt.lastExecution, now, startDate)
			if got != tt.want {
				t.Errorf("WeeklyChecker.IsDue() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMonthlyChecker_IsDue(t *testing.T) {
	checker := MonthlyChecker{}

	tests := []struct {
		name          string
		lastExecution time.Time
		now           time.Time
		startDate     core.Date
		want          bool
	}{
		{
			name:          "never executed - is due",
			lastExecution: time.Time{},
			now:           time.Date(2024, 1, 15, 12, 0, 0, 0, time.UTC),
			startDate:     core.NewDate(2024, 1, 10),
			want:          true,
		},
		{
			name:          "executed this month - not due",
			lastExecution: time.Date(2024, 1, 10, 12, 0, 0, 0, time.UTC),
			now:           time.Date(2024, 1, 15, 12, 0, 0, 0, time.UTC),
			startDate:     core.NewDate(2024, 1, 10),
			want:          false,
		},
		{
			name:          "new month but before target day - not due",
			lastExecution: time.Date(2024, 1, 15, 12, 0, 0, 0, time.UTC),
			now:           time.Date(2024, 2, 10, 12, 0, 0, 0, time.UTC),
			startDate:     core.NewDate(2024, 1, 15),
			want:          false,
		},
		{
			name:          "new month and on target day - is due",
			lastExecution: time.Date(2024, 1, 15, 12, 0, 0, 0, time.UTC),
			now:           time.Date(2024, 2, 15, 12, 0, 0, 0, time.UTC),
			startDate:     core.NewDate(2024, 1, 15),
			want:          true,
		},
		{
			name:          "target day 31 in February - adjusts to 28/29",
			lastExecution: time.Date(2024, 1, 31, 12, 0, 0, 0, time.UTC),
			now:           time.Date(2024, 2, 29, 12, 0, 0, 0, time.UTC), // 2024 is a leap year
			startDate:     core.NewDate(2024, 1, 31),
			want:          true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := checker.IsDue(tt.lastExecution, tt.now, tt.startDate)
			if got != tt.want {
				t.Errorf("MonthlyChecker.IsDue() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestYearlyChecker_IsDue(t *testing.T) {
	checker := YearlyChecker{}

	tests := []struct {
		name          string
		lastExecution time.Time
		now           time.Time
		startDate     core.Date
		want          bool
	}{
		{
			name:          "never executed - is due",
			lastExecution: time.Time{},
			now:           time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC),
			startDate:     core.NewDate(2024, 3, 15),
			want:          true,
		},
		{
			name:          "executed this year - not due",
			lastExecution: time.Date(2024, 3, 15, 12, 0, 0, 0, time.UTC),
			now:           time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC),
			startDate:     core.NewDate(2024, 3, 15),
			want:          false,
		},
		{
			name:          "new year but before target month - not due",
			lastExecution: time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC),
			now:           time.Date(2025, 3, 15, 12, 0, 0, 0, time.UTC),
			startDate:     core.NewDate(2024, 6, 15),
			want:          false,
		},
		{
			name:          "new year and past target month - is due",
			lastExecution: time.Date(2024, 3, 15, 12, 0, 0, 0, time.UTC),
			now:           time.Date(2025, 6, 15, 12, 0, 0, 0, time.UTC),
			startDate:     core.NewDate(2024, 3, 15),
			want:          true,
		},
		{
			name:          "new year same month before target day - not due",
			lastExecution: time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC),
			now:           time.Date(2025, 6, 10, 12, 0, 0, 0, time.UTC),
			startDate:     core.NewDate(2024, 6, 15),
			want:          false,
		},
		{
			name:          "new year same month on target day - is due",
			lastExecution: time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC),
			now:           time.Date(2025, 6, 15, 12, 0, 0, 0, time.UTC),
			startDate:     core.NewDate(2024, 6, 15),
			want:          true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := checker.IsDue(tt.lastExecution, tt.now, tt.startDate)
			if got != tt.want {
				t.Errorf("YearlyChecker.IsDue() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetDuenessChecker(t *testing.T) {
	tests := []struct {
		name      string
		frequency core.RepetitionTypes
		wantErr   bool
	}{
		{"daily", core.Daily, false},
		{"weekly", core.Weekly, false},
		{"monthly", core.Monthly, false},
		{"yearly", core.Yearly, false},
		{"unknown", core.RepetitionTypes("biweekly"), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			checker, err := GetDuenessChecker(tt.frequency)
			if (err != nil) != tt.wantErr {
				t.Errorf("GetDuenessChecker() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && checker == nil {
				t.Error("GetDuenessChecker() returned nil checker")
			}
		})
	}
}

func TestRegisterDuenessChecker(t *testing.T) {
	// Create a custom checker
	customChecker := DailyChecker{} // Using DailyChecker as a mock
	customFreq := core.RepetitionTypes("biweekly")

	// Register it
	RegisterDuenessChecker(customFreq, customChecker)

	// Verify it's registered
	checker, err := GetDuenessChecker(customFreq)
	if err != nil {
		t.Errorf("GetDuenessChecker() after register error = %v", err)
	}
	if checker == nil {
		t.Error("GetDuenessChecker() returned nil after registration")
	}

	// Cleanup - remove the custom checker to avoid affecting other tests
	delete(duenessStrategies, customFreq)
}
