package google

import (
    "fmt"
    "spese/internal/core"
    "strings"
)

// parseDashboard converts a values matrix (as returned by Sheets API)
// into a MonthOverview for the given year and month index (1-12).
// It expects headers including Primary, Secondary and Jan..Dec.
func parseDashboard(values [][]interface{}, year, month int) (core.MonthOverview, error) {
	if len(values) == 0 {
		return core.MonthOverview{Year: year, Month: month}, nil
	}
	headers := toStrings(values[0])
	colPrimary := indexOf(headers, "Primary")
	colSecondary := indexOf(headers, "Secondary")
	monthHeaders := []string{"Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"}
	colMonth := indexOf(headers, monthHeaders[month-1])
    if colPrimary == -1 || colSecondary == -1 || colMonth == -1 {
        // Preserve substring for upstream fallback logic; add details for troubleshooting.
        missing := make([]string, 0, 3)
        if colPrimary == -1 {
            missing = append(missing, "Primary")
        }
        if colSecondary == -1 {
            missing = append(missing, "Secondary")
        }
        if colMonth == -1 {
            missing = append(missing, monthHeaders[month-1])
        }
        return core.MonthOverview{}, fmt.Errorf("unexpected dashboard header: missing %s; got headers=%v", strings.Join(missing, ","), headers)
    }
	var totalCents int64
	byCat := map[string]int64{}
	for i := 1; i < len(values); i++ {
		row := toStrings(values[i])
		primary := safeGet(row, colPrimary)
		secondary := safeGet(row, colSecondary)
		valStr := safeGet(row, colMonth)
		if strings.EqualFold(strings.TrimSpace(primary), "total") {
			if cents, ok := parseEurosToCents(valStr); ok {
				totalCents = cents
			}
			continue
		}
		if strings.TrimSpace(primary) != "" && strings.TrimSpace(secondary) == "" {
			if cents, ok := parseEurosToCents(valStr); ok {
				byCat[strings.TrimSpace(primary)] += cents
			}
		}
	}
	if totalCents == 0 {
		for _, v := range byCat {
			totalCents += v
		}
	}
	var list []core.CategoryAmount
	seen := map[string]bool{}
	for i := 1; i < len(values); i++ {
		row := toStrings(values[i])
		primary := strings.TrimSpace(safeGet(row, colPrimary))
		secondary := strings.TrimSpace(safeGet(row, colSecondary))
		if primary == "" || secondary != "" || seen[primary] {
			continue
		}
		if amt, ok := byCat[primary]; ok {
			list = append(list, core.CategoryAmount{Name: primary, Amount: core.Money{Cents: amt}})
			seen[primary] = true
		}
	}
	for k, v := range byCat {
		if seen[k] {
			continue
		}
		list = append(list, core.CategoryAmount{Name: k, Amount: core.Money{Cents: v}})
	}
	return core.MonthOverview{Year: year, Month: month, Total: core.Money{Cents: totalCents}, ByCategory: list}, nil
}
