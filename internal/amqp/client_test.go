package amqp

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"
)

func TestExponentialBackoff(t *testing.T) {
	tests := []struct {
		attempt  int
		expected time.Duration
	}{
		{0, 1 * time.Second},
		{1, 2 * time.Second},
		{2, 4 * time.Second},
		{3, 8 * time.Second},
		{4, 16 * time.Second},
		{5, 30 * time.Second},  // capped at 30s
		{10, 30 * time.Second}, // capped at 30s
		{15, 30 * time.Second}, // capped at 30s
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("attempt_%d", tt.attempt), func(t *testing.T) {
			result := exponentialBackoff(tt.attempt)
			if result != tt.expected {
				t.Errorf("exponentialBackoff(%d) = %v, want %v", tt.attempt, result, tt.expected)
			}
		})
	}
}

func TestIsConnectionError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "nil error",
			err:      nil,
			expected: false,
		},
		{
			name:     "connection error",
			err:      errors.New("connection refused"),
			expected: true,
		},
		{
			name:     "closed connection error",
			err:      errors.New("connection closed"),
			expected: true,
		},
		{
			name:     "EOF error",
			err:      errors.New("unexpected EOF"),
			expected: true,
		},
		{
			name:     "broken pipe error",
			err:      errors.New("broken pipe"),
			expected: true,
		},
		{
			name:     "closed network connection error",
			err:      errors.New("use of closed network connection"),
			expected: true,
		},
		{
			name:     "other error",
			err:      errors.New("some other error"),
			expected: false,
		},
		{
			name:     "validation error",
			err:      errors.New("invalid input"),
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isConnectionError(tt.err)
			if result != tt.expected {
				t.Errorf("isConnectionError(%v) = %v, want %v", tt.err, result, tt.expected)
			}
		})
	}
}

func TestClient_CircuitBreaker(t *testing.T) {
	client := &Client{
		url:          "amqp://test:test@localhost:5672/",
		exchangeName: "test_exchange",
		queueName:    "test_queue",
	}

	t.Run("initial state is closed", func(t *testing.T) {
		if client.isCircuitOpen() {
			t.Error("Circuit breaker should be closed initially")
		}
	})

	t.Run("record success resets state", func(t *testing.T) {
		// Set some failures first
		atomic.StoreInt64(&client.failureCount, 3)
		atomic.StoreInt32(&client.state, StateOpen)

		client.recordSuccess()

		if client.isCircuitOpen() {
			t.Error("Circuit breaker should be closed after success")
		}
		if atomic.LoadInt64(&client.failureCount) != 0 {
			t.Error("Failure count should be reset to 0 after success")
		}
		if atomic.LoadInt32(&client.state) != StateClosed {
			t.Error("State should be StateClosed after success")
		}
	})

	t.Run("multiple failures open circuit", func(t *testing.T) {
		// Reset state
		atomic.StoreInt64(&client.failureCount, 0)
		atomic.StoreInt32(&client.state, StateClosed)

		// Record failures up to the threshold
		for i := 0; i < maxFailures; i++ {
			client.recordFailure()
		}

		if !client.isCircuitOpen() {
			t.Error("Circuit breaker should be open after max failures")
		}
		if atomic.LoadInt32(&client.state) != StateOpen {
			t.Error("State should be StateOpen after max failures")
		}
	})

	t.Run("circuit transitions to half-open after timeout", func(t *testing.T) {
		// Set circuit to open state with old timestamp
		atomic.StoreInt32(&client.state, StateOpen)
		client.lastFailure = time.Now().Add(-openTimeout - time.Second)

		// Circuit should transition to half-open
		if client.isCircuitOpen() {
			t.Error("Circuit should transition to half-open after timeout")
		}
		if atomic.LoadInt32(&client.state) != StateHalfOpen {
			t.Error("State should be StateHalfOpen after timeout")
		}
	})

	t.Run("circuit remains open within timeout", func(t *testing.T) {
		// Set circuit to open state with recent timestamp
		atomic.StoreInt32(&client.state, StateOpen)
		client.lastFailure = time.Now()

		// Circuit should remain open
		if !client.isCircuitOpen() {
			t.Error("Circuit should remain open within timeout")
		}
		if atomic.LoadInt32(&client.state) != StateOpen {
			t.Error("State should remain StateOpen within timeout")
		}
	})
}

func TestClient_PublishExpenseSync_CircuitBreaker(t *testing.T) {
	client := &Client{
		url:          "amqp://test:test@localhost:5672/",
		exchangeName: "test_exchange",
		queueName:    "test_queue",
	}

	t.Run("publish fails when circuit is open", func(t *testing.T) {
		// Set circuit to open state
		atomic.StoreInt32(&client.state, StateOpen)
		client.lastFailure = time.Now()

		ctx := context.Background()
		err := client.PublishExpenseSync(ctx, 123, 1)

		if err == nil {
			t.Error("PublishExpenseSync should fail when circuit is open")
		}
		if !contains(err.Error(), "circuit breaker is open") {
			t.Errorf("Error should mention circuit breaker, got: %v", err.Error())
		}
	})

	t.Run("publish respects context cancellation", func(t *testing.T) {
		// Reset circuit to closed state
		atomic.StoreInt32(&client.state, StateClosed)
		atomic.StoreInt64(&client.failureCount, 0)

		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately

		err := client.PublishExpenseSync(ctx, 123, 1)

		if err != context.Canceled {
			t.Errorf("PublishExpenseSync should return context.Canceled when context is cancelled, got: %v", err)
		}
	})
}

func TestNewExpenseSyncMessage(t *testing.T) {
	id := int64(12345)
	version := int64(2)

	msg := NewExpenseSyncMessage(id, version)

	if msg.ID != id {
		t.Errorf("NewExpenseSyncMessage() ID = %v, want %v", msg.ID, id)
	}
	if msg.Version != version {
		t.Errorf("NewExpenseSyncMessage() Version = %v, want %v", msg.Version, version)
	}
	if msg.Timestamp.IsZero() {
		t.Error("NewExpenseSyncMessage() Timestamp should not be zero")
	}
	if time.Since(msg.Timestamp) > time.Second {
		t.Error("NewExpenseSyncMessage() Timestamp should be recent")
	}
}

func TestExpenseSyncMessage_JSON(t *testing.T) {
	timestamp := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	msg := &ExpenseSyncMessage{
		ID:        12345,
		Version:   2,
		Timestamp: timestamp,
	}

	// Test JSON marshaling
	jsonBytes, err := msg.ToJSON()
	if err != nil {
		t.Fatalf("ToJSON() error = %v", err)
	}

	// Test JSON unmarshaling
	parsedMsg, err := ExpenseSyncMessageFromJSON(jsonBytes)
	if err != nil {
		t.Fatalf("ExpenseSyncMessageFromJSON() error = %v", err)
	}

	if parsedMsg.ID != msg.ID {
		t.Errorf("Parsed ID = %v, want %v", parsedMsg.ID, msg.ID)
	}
	if parsedMsg.Version != msg.Version {
		t.Errorf("Parsed Version = %v, want %v", parsedMsg.Version, msg.Version)
	}
	if !parsedMsg.Timestamp.Equal(msg.Timestamp) {
		t.Errorf("Parsed Timestamp = %v, want %v", parsedMsg.Timestamp, msg.Timestamp)
	}
}

func TestExpenseSyncMessage_InvalidJSON(t *testing.T) {
	invalidJSON := []byte(`{"id": "not_a_number", "version": 1}`)

	_, err := ExpenseSyncMessageFromJSON(invalidJSON)
	if err == nil {
		t.Error("ExpenseSyncMessageFromJSON() should fail with invalid JSON")
	}
}

// Helper function for string contains check (same as in config_test.go)
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || func() bool {
		for i := 0; i <= len(s)-len(substr); i++ {
			if s[i:i+len(substr)] == substr {
				return true
			}
		}
		return false
	}())
}
