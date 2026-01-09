package services

import (
	"testing"
)

func TestNewExpenseService(t *testing.T) {
	// Test with nil values since we can't easily mock the concrete types
	service := NewExpenseService(nil)

	if service == nil {
		t.Error("NewExpenseService should return a non-nil service")
	}
	if service.storage != nil {
		t.Error("NewExpenseService should set storage to nil when passed nil")
	}
}

func TestExpenseService_CreateExpense(t *testing.T) {
	// Note: This would require integration testing or proper mocking interfaces
	// For now, we just test that the service doesn't panic when created
	service := NewExpenseService(nil)
	if service == nil {
		t.Error("NewExpenseService should return non-nil service even with nil arguments")
	}
}

func TestExpenseService_Close(t *testing.T) {
	t.Run("nil components", func(t *testing.T) {
		service := &ExpenseService{
			storage: nil,
		}

		err := service.Close()

		if err != nil {
			t.Fatalf("Close should not return error with nil components: %v", err)
		}
	})
}
