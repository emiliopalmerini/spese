package ledger

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"spese/internal/storage"
)

func TestMovementInvariantsAndBalances(t *testing.T) {
	ctx := context.Background()
	svc := newTestService(t)
	wallet := mustAccount(t, svc, AccountInput{Name: "Conto", Type: AccountAsset, Class: "Cash", InitialBalanceCents: 100_00, InitialDate: "2026-01-01"})
	card := mustAccount(t, svc, AccountInput{Name: "Carta", Type: AccountLiability, Class: "Credit", InitialBalanceCents: -20_00, InitialDate: "2026-01-01"})
	food := mustCategory(t, svc, CategoryInput{Name: "Alimentari", Kind: CategoryExpense, Color: "#2E6F95", Icon: "shopping-basket"})
	salary := mustCategory(t, svc, CategoryInput{Name: "Stipendio", Kind: CategoryIncome, Color: "#6A7B35", Icon: "briefcase"})

	tests := []struct {
		name  string
		input MovementInput
		want  map[string]int64
	}{
		{
			name: "expense debits asset",
			input: MovementInput{Kind: MovementExpense, Status: MovementPosted, Date: "2026-02-02", AccountID: wallet.ID, AmountCents: 25_00,
				Merchant: "Forno", Allocations: []AllocationInput{{CategoryID: food.ID, AmountCents: 25_00}}},
			want: map[string]int64{wallet.ID: 75_00},
		},
		{
			name: "income credits asset",
			input: MovementInput{Kind: MovementIncome, Status: MovementPosted, Date: "2026-02-03", AccountID: wallet.ID, AmountCents: 50_00,
				Allocations: []AllocationInput{{CategoryID: salary.ID, AmountCents: 50_00}}},
			want: map[string]int64{wallet.ID: 125_00},
		},
		{
			name:  "transfer is zero sum",
			input: MovementInput{Kind: MovementTransfer, Status: MovementPosted, Date: "2026-02-04", AccountID: wallet.ID, DestinationAccountID: card.ID, AmountCents: 10_00},
			want:  map[string]int64{wallet.ID: 115_00, card.ID: -10_00},
		},
		{
			name: "refund reduces original expense",
			input: MovementInput{Kind: MovementRefund, Status: MovementPosted, Date: "2026-02-05", AccountID: wallet.ID, AmountCents: 5_00,
				Merchant: "Forno", Allocations: []AllocationInput{{CategoryID: food.ID, AmountCents: 5_00}}},
			want: map[string]int64{wallet.ID: 120_00},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			movement, err := svc.CreateMovement(ctx, tt.input)
			if err != nil {
				t.Fatalf("CreateMovement: %v", err)
			}
			var sum int64
			for _, posting := range movement.Postings {
				sum += posting.AmountCents
			}
			if tt.input.Kind == MovementTransfer && sum != 0 {
				t.Fatalf("transfer posting sum = %d, want 0", sum)
			}
			for accountID, want := range tt.want {
				balance, err := svc.Balance(ctx, accountID, "2026-02-28")
				if err != nil {
					t.Fatalf("Balance: %v", err)
				}
				if balance.BalanceCents != want {
					t.Fatalf("balance %s = %d, want %d", accountID, balance.BalanceCents, want)
				}
			}
		})
	}
}

func TestMovementRejectsInvalidAllocationsAndTransferCategory(t *testing.T) {
	ctx := context.Background()
	svc := newTestService(t)
	a := mustAccount(t, svc, AccountInput{Name: "A", Type: AccountAsset, Class: "Cash", InitialDate: "2026-01-01"})
	b := mustAccount(t, svc, AccountInput{Name: "B", Type: AccountAsset, Class: "Cash", InitialDate: "2026-01-01"})
	category := mustCategory(t, svc, CategoryInput{Name: "Casa", Kind: CategoryExpense, Color: "#865D36", Icon: "house"})

	_, err := svc.CreateMovement(ctx, MovementInput{Kind: MovementExpense, Status: MovementPosted, Date: "2026-02-01", AccountID: a.ID, AmountCents: 10_00,
		Allocations: []AllocationInput{{CategoryID: category.ID, AmountCents: 9_00}}})
	if !errors.Is(err, ErrAllocationMismatch) {
		t.Fatalf("split error = %v, want ErrAllocationMismatch", err)
	}

	_, err = svc.CreateMovement(ctx, MovementInput{Kind: MovementTransfer, Status: MovementPosted, Date: "2026-02-01", AccountID: a.ID, DestinationAccountID: b.ID, AmountCents: 10_00,
		Allocations: []AllocationInput{{CategoryID: category.ID, AmountCents: 10_00}}})
	if !errors.Is(err, ErrTransferAllocation) {
		t.Fatalf("transfer allocation error = %v, want ErrTransferAllocation", err)
	}
}

func TestMovementKeepsCounterpartyAndDescriptionSeparate(t *testing.T) {
	svc := newTestService(t)
	account := mustAccount(t, svc, AccountInput{Name: "Conto", Type: AccountAsset, Class: "Cash", InitialDate: "2026-01-01"})
	category := mustCategory(t, svc, CategoryInput{Name: "Lavoro", Kind: CategoryIncome})
	movement := mustMovement(t, svc, MovementInput{
		Kind: MovementIncome, Status: MovementPosted, Date: "2026-02-01", AccountID: account.ID,
		AmountCents: 100_00, Merchant: "Cliente Acme", Description: "Saldo fattura 42",
		Allocations: []AllocationInput{{CategoryID: category.ID, AmountCents: 100_00}},
	})
	if movement.Merchant != "Cliente Acme" || movement.Description != "Saldo fattura 42" {
		t.Fatalf("merchant/description collapsed: %#v", movement)
	}
}

func TestDraftAndVoidDoNotAffectBalance(t *testing.T) {
	ctx := context.Background()
	svc := newTestService(t)
	account := mustAccount(t, svc, AccountInput{Name: "Conto", Type: AccountAsset, Class: "Cash", InitialBalanceCents: 50_00, InitialDate: "2026-01-01"})
	category := mustCategory(t, svc, CategoryInput{Name: "Varie", Kind: CategoryExpense, Color: "#725B86", Icon: "shapes"})

	draft, err := svc.CreateMovement(ctx, MovementInput{Kind: MovementExpense, Status: MovementDraft, Date: "2026-02-01", AccountID: account.ID, AmountCents: 10_00,
		Allocations: []AllocationInput{{CategoryID: category.ID, AmountCents: 10_00}}})
	if err != nil {
		t.Fatal(err)
	}
	assertBalance(t, svc, account.ID, "2026-02-28", 50_00)

	posted, err := svc.UpdateMovement(ctx, draft.ID, draft.Version, MovementInput{Kind: MovementExpense, Status: MovementPosted, Date: "2026-02-01", AccountID: account.ID, AmountCents: 10_00,
		Allocations: []AllocationInput{{CategoryID: category.ID, AmountCents: 10_00}}})
	if err != nil {
		t.Fatal(err)
	}
	assertBalance(t, svc, account.ID, "2026-02-28", 40_00)

	voided, err := svc.VoidMovement(ctx, posted.ID, posted.Version, "Inserimento errato")
	if err != nil {
		t.Fatal(err)
	}
	if voided.Status != MovementVoid || voided.VoidedAt == "" {
		t.Fatalf("movement was not audibly voided: %#v", voided)
	}
	assertBalance(t, svc, account.ID, "2026-02-28", 50_00)
}

func TestOptimisticLocking(t *testing.T) {
	ctx := context.Background()
	svc := newTestService(t)
	account := mustAccount(t, svc, AccountInput{Name: "Conto", Type: AccountAsset, Class: "Cash", InitialDate: "2026-01-01"})
	category := mustCategory(t, svc, CategoryInput{Name: "Casa", Kind: CategoryExpense, Color: "#865D36", Icon: "house"})
	movement, err := svc.CreateMovement(ctx, MovementInput{Kind: MovementExpense, Status: MovementPosted, Date: "2026-02-01", AccountID: account.ID, AmountCents: 10_00,
		Allocations: []AllocationInput{{CategoryID: category.ID, AmountCents: 10_00}}})
	if err != nil {
		t.Fatal(err)
	}
	_, err = svc.UpdateMovement(ctx, movement.ID, movement.Version+1, MovementInput{Kind: MovementExpense, Status: MovementPosted, Date: "2026-02-01", AccountID: account.ID, AmountCents: 11_00,
		Allocations: []AllocationInput{{CategoryID: category.ID, AmountCents: 11_00}}})
	if !errors.Is(err, ErrVersionConflict) {
		t.Fatalf("UpdateMovement error = %v, want ErrVersionConflict", err)
	}
}

func TestReconciliationAnchorsFutureBalance(t *testing.T) {
	ctx := context.Background()
	svc := newTestService(t)
	account := mustAccount(t, svc, AccountInput{Name: "Conto", Type: AccountAsset, Class: "Cash", InitialBalanceCents: 100_00, InitialDate: "2026-01-01"})
	category := mustCategory(t, svc, CategoryInput{Name: "Casa", Kind: CategoryExpense, Color: "#865D36", Icon: "house"})
	mustMovement(t, svc, MovementInput{Kind: MovementExpense, Status: MovementPosted, Date: "2026-01-20", AccountID: account.ID, AmountCents: 20_00,
		Allocations: []AllocationInput{{CategoryID: category.ID, AmountCents: 20_00}}})

	preview, err := svc.PreviewReconciliation(ctx, "2026-01", []ReconciliationInput{{AccountID: account.ID, ActualBalanceCents: 82_00}})
	if err != nil {
		t.Fatal(err)
	}
	if preview.Accounts[0].ExpectedBalanceCents != 80_00 || preview.Accounts[0].DifferenceCents != 2_00 {
		t.Fatalf("unexpected preview: %#v", preview.Accounts[0])
	}
	if _, err := svc.CommitReconciliation(ctx, preview); err != nil {
		t.Fatal(err)
	}
	mustMovement(t, svc, MovementInput{Kind: MovementExpense, Status: MovementPosted, Date: "2026-02-10", AccountID: account.ID, AmountCents: 10_00,
		Allocations: []AllocationInput{{CategoryID: category.ID, AmountCents: 10_00}}})
	assertBalance(t, svc, account.ID, "2026-02-28", 72_00)

	// Historical analytics change, while the authoritative January anchor keeps
	// the subsequent balance deterministic.
	mustMovement(t, svc, MovementInput{Kind: MovementExpense, Status: MovementPosted, Date: "2026-01-10", AccountID: account.ID, AmountCents: 5_00,
		Allocations: []AllocationInput{{CategoryID: category.ID, AmountCents: 5_00}}})
	assertBalance(t, svc, account.ID, "2026-02-28", 72_00)
}

func TestRecurringCatchUpLastDayAndConcurrency(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, time.April, 30, 12, 0, 0, 0, time.UTC)
	svc := newTestService(t)
	svc.now = func() time.Time { return now }
	account := mustAccount(t, svc, AccountInput{Name: "Conto", Type: AccountAsset, Class: "Cash", InitialDate: "2026-01-01"})
	category := mustCategory(t, svc, CategoryInput{Name: "Affitto", Kind: CategoryExpense, Color: "#865D36", Icon: "house"})
	rule, err := svc.CreateRecurringRule(ctx, RecurringRuleInput{
		Kind: MovementExpense, Frequency: FrequencyMonthly, Interval: 1, StartDate: "2026-01-31", DayOfMonth: 31,
		Timezone: "Europe/Rome", AmountCents: 10_00, AccountID: account.ID, CategoryID: category.ID,
		Merchant: "Affitto", Mode: RecurringAutoPost, AmountMode: RecurringFixed,
	})
	if err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	for range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := svc.ProcessRecurring(ctx, now); err != nil {
				t.Errorf("ProcessRecurring: %v", err)
			}
		}()
	}
	wg.Wait()

	occurrences, err := svc.ListOccurrences(ctx, rule.ID)
	if err != nil {
		t.Fatal(err)
	}
	wantDates := []string{"2026-01-31", "2026-02-28", "2026-03-31", "2026-04-30"}
	if len(occurrences) != len(wantDates) {
		t.Fatalf("occurrences = %d, want %d: %#v", len(occurrences), len(wantDates), occurrences)
	}
	for i, want := range wantDates {
		if occurrences[i].ScheduledFor != want || occurrences[i].MovementID == "" || occurrences[i].Status != OccurrencePosted {
			t.Fatalf("occurrence %d = %#v, want posted %s", i, occurrences[i], want)
		}
	}
	assertBalance(t, svc, account.ID, "2026-04-30", -40_00)
}

func newTestService(t *testing.T) *Service {
	t.Helper()
	store, err := storage.Open(context.Background(), "file:"+t.Name()+"?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return New(store)
}

func mustAccount(t *testing.T, svc *Service, input AccountInput) Account {
	t.Helper()
	account, err := svc.CreateAccount(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	return account
}

func mustCategory(t *testing.T, svc *Service, input CategoryInput) Category {
	t.Helper()
	category, err := svc.CreateCategory(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	return category
}

func mustMovement(t *testing.T, svc *Service, input MovementInput) Movement {
	t.Helper()
	movement, err := svc.CreateMovement(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	return movement
}

func assertBalance(t *testing.T, svc *Service, accountID, asOf string, want int64) {
	t.Helper()
	balance, err := svc.Balance(context.Background(), accountID, asOf)
	if err != nil {
		t.Fatal(err)
	}
	if balance.BalanceCents != want {
		t.Fatalf("balance = %d, want %d", balance.BalanceCents, want)
	}
}
