package amqp

import (
	"encoding/json"
	"time"
)

// ExpenseSyncMessage represents a lightweight message for syncing an expense to Google Sheets
// Contains only the ID and version, the worker will fetch the full expense from database
type ExpenseSyncMessage struct {
	ID        int64     `json:"id"`
	Version   int64     `json:"version"`
	Timestamp time.Time `json:"timestamp"`
}

// NewExpenseSyncMessage creates a new sync message with just ID and version
func NewExpenseSyncMessage(id, version int64) *ExpenseSyncMessage {
	return &ExpenseSyncMessage{
		ID:        id,
		Version:   version,
		Timestamp: time.Now(),
	}
}

// ToJSON converts the message to JSON bytes
func (m *ExpenseSyncMessage) ToJSON() ([]byte, error) {
	return json.Marshal(m)
}

// FromJSON creates a message from JSON bytes
func ExpenseSyncMessageFromJSON(data []byte) (*ExpenseSyncMessage, error) {
	var msg ExpenseSyncMessage
	if err := json.Unmarshal(data, &msg); err != nil {
		return nil, err
	}
	return &msg, nil
}

// ExpenseDeleteMessage represents a message for deleting an expense from Google Sheets
// Includes expense data to identify the row in Google Sheets since it's deleted from SQLite first
type ExpenseDeleteMessage struct {
	ID          int64     `json:"id"`
	Day         int       `json:"day"`
	Month       int       `json:"month"`
	Description string    `json:"description"`
	AmountCents int64     `json:"amount_cents"`
	Primary     string    `json:"primary"`
	Secondary   string    `json:"secondary"`
	Timestamp   time.Time `json:"timestamp"`
}

// NewExpenseDeleteMessage creates a new delete message with expense data
func NewExpenseDeleteMessage(id int64, day, month int, description string, amountCents int64, primary, secondary string) *ExpenseDeleteMessage {
	return &ExpenseDeleteMessage{
		ID:          id,
		Day:         day,
		Month:       month,
		Description: description,
		AmountCents: amountCents,
		Primary:     primary,
		Secondary:   secondary,
		Timestamp:   time.Now(),
	}
}

// ToJSON converts the delete message to JSON bytes
func (m *ExpenseDeleteMessage) ToJSON() ([]byte, error) {
	return json.Marshal(m)
}

// ExpenseDeleteMessageFromJSON creates a delete message from JSON bytes
func ExpenseDeleteMessageFromJSON(data []byte) (*ExpenseDeleteMessage, error) {
	var msg ExpenseDeleteMessage
	if err := json.Unmarshal(data, &msg); err != nil {
		return nil, err
	}
	return &msg, nil
}
