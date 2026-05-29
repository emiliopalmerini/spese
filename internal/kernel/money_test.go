package kernel

import "testing"

func TestParseMoney(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want Money
	}{
		{name: "plain integer", in: "1234", want: 123400},
		{name: "english decimal", in: "1234.56", want: 123456},
		{name: "italian decimal", in: "1234,56", want: 123456},
		{name: "english thousands", in: "1,234.56", want: 123456},
		{name: "italian thousands", in: "1.234,56", want: 123456},
		{name: "formatted euro", in: "1.234,56 €", want: 123456},
		{name: "negative italian thousands", in: "-1.234,56", want: -123456},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseMoney(tt.in)
			if err != nil {
				t.Fatalf("ParseMoney(%q) returned error: %v", tt.in, err)
			}
			if got != tt.want {
				t.Fatalf("ParseMoney(%q) = %d, want %d", tt.in, got, tt.want)
			}
		})
	}
}
