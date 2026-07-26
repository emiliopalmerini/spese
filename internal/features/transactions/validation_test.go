package transactions

import (
	"testing"

	"spese/internal/kernel"
)

func TestValidateInput(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   Input
		want    Transaction
		wantErr string
	}{
		{
			name: "expense gets negative amount",
			input: Input{
				Kind:    "Expense",
				Date:    "2026-07-26",
				Account: "Fineco",
				Amount:  "12,50",
				Payee:   "Conad",
			},
			want: Transaction{
				Kind:    Expense,
				Date:    mustDate(t, "2026-07-26"),
				Account: "Fineco",
				Amount:  kernel.Money(-1250),
				Payee:   "Conad",
			},
		},
		{
			name: "income normalizes negative input",
			input: Input{
				Kind:    "Income",
				Date:    "2026-07-26",
				Account: "Fineco",
				Amount:  "-1.234,56",
				Payee:   "Stipendio",
			},
			want: Transaction{
				Kind:    Income,
				Date:    mustDate(t, "2026-07-26"),
				Account: "Fineco",
				Amount:  kernel.Money(123456),
				Payee:   "Stipendio",
			},
		},
		{
			name:    "rejects unsupported kind",
			input:   Input{Kind: "Transfer"},
			wantErr: "Seleziona un tipo di movimento valido.",
		},
		{
			name:    "requires account",
			input:   Input{Kind: "Expense"},
			wantErr: "Seleziona un conto.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := ValidateInput(tt.input)
			if tt.wantErr != "" {
				if err == nil || err.Error() != tt.wantErr {
					t.Fatalf("ValidateInput() error = %v, want %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("ValidateInput() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("ValidateInput() = %#v, want %#v", got, tt.want)
			}
		})
	}
}
