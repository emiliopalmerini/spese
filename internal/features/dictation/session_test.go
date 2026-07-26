package dictation

import (
	"context"
	"testing"
	"time"
)

type fakeExtractor struct {
	requests []ExtractRequest
	outputs  []Extraction
	deleted  string
}

func (f *fakeExtractor) CreateSession(context.Context) (string, error) {
	return "session-1", nil
}

func (f *fakeExtractor) Extract(_ context.Context, _ string, request ExtractRequest) (Extraction, error) {
	f.requests = append(f.requests, request)
	output := f.outputs[0]
	f.outputs = f.outputs[1:]
	return NormalizeExtraction(request.Previous, output), nil
}

func (f *fakeExtractor) DeleteSession(_ context.Context, id string) error {
	f.deleted = id
	return nil
}

func TestCaptureCarriesDraftStateAcrossCorrectionsAndDeletesSession(t *testing.T) {
	t.Parallel()

	extractor := &fakeExtractor{outputs: []Extraction{
		{Movements: []Draft{{Kind: "Expense", Date: "2026-07-26", Account: "Fineco", Amount: "12,50", Payee: "Conad"}}},
		{Movements: []Draft{{ID: "draft-1", Kind: "Expense", Date: "2026-07-26", Account: "Fineco", Amount: "24,00", Payee: "Conad"}}, FinishRequested: true},
	}}
	capture, err := StartCapture(context.Background(), extractor, CaptureContext{
		Today: time.Date(2026, 7, 26, 0, 0, 0, 0, time.UTC), Accounts: []string{"Fineco"},
	})
	if err != nil {
		t.Fatal(err)
	}

	first, err := capture.Apply(context.Background(), "Dodici euro e cinquanta da Conad")
	if err != nil {
		t.Fatal(err)
	}
	if first.Movements[0].ID != "draft-1" {
		t.Fatalf("first id = %q", first.Movements[0].ID)
	}
	second, err := capture.Apply(context.Background(), "Anzi erano ventiquattro euro, e basta")
	if err != nil {
		t.Fatal(err)
	}
	if second.Movements[0].Amount != "24,00" || !second.FinishRequested {
		t.Fatalf("second = %#v", second)
	}
	if len(extractor.requests[1].Previous) != 1 || extractor.requests[1].Previous[0].ID != "draft-1" {
		t.Fatalf("previous state = %#v", extractor.requests[1].Previous)
	}
	if err := capture.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	if extractor.deleted != "session-1" {
		t.Fatalf("deleted = %q", extractor.deleted)
	}
}
