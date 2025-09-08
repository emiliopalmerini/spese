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
