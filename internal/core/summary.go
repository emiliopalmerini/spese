package core

// CategoryAmount represents an amount aggregated by category name.
type CategoryAmount struct {
    Name   string
    Amount Money
}

// MonthOverview is a compact summary for a specific year+month.
type MonthOverview struct {
    Year       int
    Month      int // 1-12
    Total      Money
    ByCategory []CategoryAmount
}

