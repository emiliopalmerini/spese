package google

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"

	"spese/internal/core"
	ports "spese/internal/sheets"

	gsheet "google.golang.org/api/sheets/v4"
)

var _ ports.NetWorthWriter = (*Client)(nil)

// Section title aliases. Keys are normalized labels used in the dashboard sheet
// (case-insensitive); values are the domain account types.
var nwSectionAliases = map[string]core.AccountType{
	"cash - liquidity":     core.AccountCash,
	"cash & liquidità":     core.AccountCash,
	"liquidity":            core.AccountCash,
	"rain day funds":       core.AccountRainyDay,
	"rainy day funds":      core.AccountRainyDay,
	"rainy day":            core.AccountRainyDay,
	"long term investment": core.AccountLongTerm,
	"long term":            core.AccountLongTerm,
}

// nwAccountRow locates an account row inside the dashboard sheet.
type nwAccountRow struct {
	Name string
	Row  int // 1-indexed
}

// nwSection groups account rows for a section in the Net Worth table.
type nwSection struct {
	Type      core.AccountType
	Title     string
	HeaderRow int // 1-indexed row number of section header
	BlankRow  int // 1-indexed row index after the last account (trailing blank)
	Accounts  []nwAccountRow
}

// nwLayout describes the parsed layout of the Net Worth section of the sheet.
type nwLayout struct {
	HeaderRowIndex int // 1-indexed row containing year/month headers
	NetWorthRow    int // 1-indexed row containing the "Net Worth" cell
	Sections       []nwSection
}

// parseNetWorthLayout parses the dashboard sheet contents to locate the Net
// Worth section, its sub-sections, and account rows. The grid is the column-A
// to column-Z block; rowOffset is the 1-indexed row number that grid[0]
// represents (typically 1).
func parseNetWorthLayout(grid [][]string, rowOffset int) (nwLayout, error) {
	var out nwLayout

	// Locate the row containing "Net Worth" in column A.
	netWorthIdx := -1
	for i, row := range grid {
		if len(row) == 0 {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(row[0]), "net worth") {
			netWorthIdx = i
			break
		}
	}
	if netWorthIdx == -1 {
		return out, errors.New("net worth header not found")
	}
	out.NetWorthRow = netWorthIdx + rowOffset

	// Find the most recent header row above netWorthIdx that contains year/month columns.
	// We assume the same row contains year headers and month abbreviations.
	headerIdx := findHeaderRow(grid, netWorthIdx)
	if headerIdx == -1 {
		return out, errors.New("year/month header row not found above Net Worth")
	}
	out.HeaderRowIndex = headerIdx + rowOffset

	// Walk forward from netWorthIdx+1, alternating section headers and account
	// rows separated by blank rows.
	i := netWorthIdx + 1
	for i < len(grid) {
		row := grid[i]
		label := ""
		if len(row) > 0 {
			label = strings.TrimSpace(row[0])
		}
		if label == "" {
			i++
			continue
		}
		t, isSection := nwSectionAliases[strings.ToLower(label)]
		if !isSection {
			// Not a known section header. Stop scanning the Net Worth block.
			break
		}
		section := nwSection{
			Type:      t,
			Title:     label,
			HeaderRow: i + rowOffset,
		}
		j := i + 1
		for j < len(grid) {
			r := grid[j]
			name := ""
			if len(r) > 0 {
				name = strings.TrimSpace(r[0])
			}
			if name == "" {
				break
			}
			// If we hit another section header, stop without consuming it.
			if _, ok := nwSectionAliases[strings.ToLower(name)]; ok {
				break
			}
			section.Accounts = append(section.Accounts, nwAccountRow{
				Name: name,
				Row:  j + rowOffset,
			})
			j++
		}
		section.BlankRow = j + rowOffset
		out.Sections = append(out.Sections, section)
		i = j + 1
	}

	if len(out.Sections) == 0 {
		return out, errors.New("no Net Worth sections parsed")
	}
	return out, nil
}

// findHeaderRow returns the index of the closest row above netWorthIdx that
// looks like a year/month header (contains a 4-digit year and at least one
// month abbreviation).
func findHeaderRow(grid [][]string, netWorthIdx int) int {
	for i := netWorthIdx - 1; i >= 0; i-- {
		row := grid[i]
		hasYear := false
		hasMonth := false
		for _, c := range row {
			c = strings.TrimSpace(c)
			if isYearLabel(c) {
				hasYear = true
			}
			if isMonthLabel(c) {
				hasMonth = true
			}
			if hasYear && hasMonth {
				return i
			}
		}
	}
	return -1
}

func isYearLabel(s string) bool {
	if len(s) != 4 {
		return false
	}
	y, err := strconv.Atoi(s)
	if err != nil {
		return false
	}
	return y >= 2000 && y < 2999
}

var monthLabels = map[string]int{
	"jan": 1, "feb": 2, "mar": 3, "apr": 4, "may": 5, "jun": 6,
	"jul": 7, "aug": 8, "sep": 9, "oct": 10, "nov": 11, "dec": 12,
	"gen": 1, "feb.": 2, "mar.": 3, "apr.": 4, "mag": 5, "giu": 6,
	"lug": 7, "ago": 8, "set": 9, "ott": 10, "nov.": 11, "dic": 12,
}

func isMonthLabel(s string) bool {
	_, ok := monthLabels[strings.ToLower(strings.TrimSpace(s))]
	return ok
}

// findMonthColumn returns the 0-indexed column index for the requested
// (year, month) by scanning the header row. The header is expected to contain
// a year cell (e.g. "2025") followed by month cells "Jan".."Dec", repeated for
// each year.
func findMonthColumn(header []string, year, month int) (int, error) {
	target := strconv.Itoa(year)
	for i, c := range header {
		if strings.TrimSpace(c) == target {
			// Walk forward 12 cells to find the month abbreviation.
			for j := i + 1; j <= i+12 && j < len(header); j++ {
				m, ok := monthLabels[strings.ToLower(strings.TrimSpace(header[j]))]
				if !ok {
					continue
				}
				if m == month {
					return j, nil
				}
			}
			return 0, fmt.Errorf("month %d not found under year %d header", month, year)
		}
	}
	return 0, fmt.Errorf("year %d not found in header", year)
}

// columnLetter converts a 0-indexed column number into spreadsheet letters
// (0 → "A", 25 → "Z", 26 → "AA", ...).
func columnLetter(idx int) string {
	if idx < 0 {
		return ""
	}
	idx++
	out := ""
	for idx > 0 {
		idx--
		out = string(rune('A'+(idx%26))) + out
		idx /= 26
	}
	return out
}

// UpsertBalance writes an account's balance at the row/column derived from the
// dashboard sheet's Net Worth layout. Implements ports.NetWorthWriter.
func (c *Client) UpsertBalance(ctx context.Context, accountName string, accountType core.AccountType,
	year, month int, amount core.Money) (string, error) {
	if c.svc == nil {
		return "", errors.New("sheets service not initialized")
	}
	sheetName := c.dashboardSheetForYear(year)
	if sheetName == "" {
		return "", errors.New("dashboard sheet name not configured")
	}

	rng := fmt.Sprintf("%s!A1:Z200", sheetName)
	resp, err := c.svc.Spreadsheets.Values.Get(c.spreadsheetID, rng).Context(ctx).Do()
	if err != nil {
		return "", fmt.Errorf("read dashboard sheet %q: %w", sheetName, err)
	}

	grid := valueRangeToGrid(resp)
	layout, err := parseNetWorthLayout(grid, 1)
	if err != nil {
		return "", fmt.Errorf("parse layout: %w", err)
	}

	headerRow := grid[layout.HeaderRowIndex-1]
	colIdx, err := findMonthColumn(headerRow, year, month)
	if err != nil {
		return "", err
	}

	var section *nwSection
	for i := range layout.Sections {
		if layout.Sections[i].Type == accountType {
			section = &layout.Sections[i]
			break
		}
	}
	if section == nil {
		return "", fmt.Errorf("section for type %q not found in sheet", accountType)
	}

	rowIdx := -1
	for _, a := range section.Accounts {
		if strings.EqualFold(a.Name, accountName) {
			rowIdx = a.Row
			break
		}
	}
	if rowIdx == -1 {
		// Append a new row at the end of the section. We write the name and
		// the value in the same call. The trailing blank row receives the new
		// account so existing layout is preserved.
		rowIdx = section.BlankRow
		nameRange := fmt.Sprintf("%s!A%d", sheetName, rowIdx)
		_, err := c.svc.Spreadsheets.Values.Update(c.spreadsheetID, nameRange, &gsheet.ValueRange{
			Values: [][]any{{accountName}},
		}).ValueInputOption("USER_ENTERED").Context(ctx).Do()
		if err != nil {
			return "", fmt.Errorf("append account row: %w", err)
		}
	}

	cell := fmt.Sprintf("%s!%s%d", sheetName, columnLetter(colIdx), rowIdx)
	euros := float64(amount.Cents) / 100.0
	_, err = c.svc.Spreadsheets.Values.Update(c.spreadsheetID, cell, &gsheet.ValueRange{
		Values: [][]any{{euros}},
	}).ValueInputOption("USER_ENTERED").Context(ctx).Do()
	if err != nil {
		return "", fmt.Errorf("write balance cell %s: %w", cell, err)
	}

	slog.InfoContext(ctx, "Net worth balance written to sheet",
		"sheet", sheetName,
		"cell", cell,
		"account", accountName,
		"amount_eur", euros)

	return cell, nil
}

// valueRangeToGrid converts the *ValueRange response into a [][]string grid.
func valueRangeToGrid(resp *gsheet.ValueRange) [][]string {
	if resp == nil {
		return nil
	}
	out := make([][]string, len(resp.Values))
	for i, row := range resp.Values {
		cells := make([]string, len(row))
		for j, v := range row {
			cells[j] = fmt.Sprintf("%v", v)
		}
		out[i] = cells
	}
	return out
}

// dashboardSheetForYear returns the dashboard sheet name for the given year,
// preferring the configured base name and falling back to the legacy prefix.
func (c *Client) dashboardSheetForYear(year int) string {
	if c.dashboardBase != "" {
		return yearPrefixedName(c.dashboardBase, year)
	}
	if c.dashboardPrefix != "" {
		// Legacy: prefix string may be a Printf pattern like "%d Dashboard"
		if strings.Contains(c.dashboardPrefix, "%d") {
			return fmt.Sprintf(c.dashboardPrefix, year)
		}
		return fmt.Sprintf("%d %s", year, c.dashboardPrefix)
	}
	return ""
}
