package sheets

import (
	"math"
	"testing"

	"spese/internal/kernel"
)

func TestCellMoneyParsesFormattedThousands(t *testing.T) {
	tests := []struct {
		name string
		in   any
		want kernel.Money
	}{
		{name: "english thousands euro", in: "5,086 €", want: 508600},
		{name: "italian thousands euro", in: "5.086 €", want: 508600},
		{name: "english thousands dollar", in: "$74,746", want: 7474600},
		{name: "negative thousands", in: "-6,577 €", want: -657700},
		{name: "english decimal", in: "1,234.56 €", want: 123456},
		{name: "italian decimal", in: "1.234,56 €", want: 123456},
		{name: "decimal comma cents", in: "12,34 €", want: 1234},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := CellMoney(tt.in)
			if !ok {
				t.Fatalf("CellMoney(%q) did not parse", tt.in)
			}
			if got != tt.want {
				t.Fatalf("CellMoney(%q) = %d, want %d", tt.in, got, tt.want)
			}
		})
	}
}

func TestCellFloatParsesFormattedNumbers(t *testing.T) {
	tests := []struct {
		name string
		in   any
		want float64
	}{
		{name: "english thousands", in: "5,086 €", want: 5086},
		{name: "italian thousands", in: "5.086 €", want: 5086},
		{name: "english decimal", in: "1,234.56", want: 1234.56},
		{name: "italian decimal", in: "1.234,56", want: 1234.56},
		{name: "percent dot", in: "83.7%", want: 0.837},
		{name: "percent comma", in: "83,7%", want: 0.837},
		{name: "ratio with three decimals", in: "0.837", want: 0.837},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := CellFloat(tt.in)
			if !ok {
				t.Fatalf("CellFloat(%q) did not parse", tt.in)
			}
			if math.Abs(got-tt.want) > 0.000001 {
				t.Fatalf("CellFloat(%q) = %f, want %f", tt.in, got, tt.want)
			}
		})
	}
}
