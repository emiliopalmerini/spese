package dictation

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"
)

type extractor interface {
	CreateSession(context.Context) (string, error)
	Extract(context.Context, string, ExtractRequest) (Extraction, error)
	DeleteSession(context.Context, string) error
}

// CaptureContext is the bounded accounting context fixed for one recording.
type CaptureContext struct {
	Today      time.Time
	Accounts   []string
	Categories []string
	History    []HistoryItem
}

// Capture serializes model turns and owns one ephemeral OpenCode session.
type Capture struct {
	mu        sync.Mutex
	extractor extractor
	sessionID string
	context   CaptureContext
	state     Extraction
	closed    bool
}

func StartCapture(ctx context.Context, extractor extractor, captureContext CaptureContext) (*Capture, error) {
	if extractor == nil {
		return nil, errors.New("dictation: extractor is required")
	}
	sessionID, err := extractor.CreateSession(ctx)
	if err != nil {
		return nil, err
	}
	return &Capture{extractor: extractor, sessionID: sessionID, context: captureContext}, nil
}

func (c *Capture) Apply(ctx context.Context, transcript string) (Extraction, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return Extraction{}, errors.New("dictation: capture is closed")
	}
	transcript = strings.TrimSpace(transcript)
	if transcript == "" {
		return c.state, nil
	}
	next, err := c.extractor.Extract(ctx, c.sessionID, ExtractRequest{
		Today:      c.context.Today,
		Transcript: transcript,
		Previous:   append([]Draft(nil), c.state.Movements...),
		Accounts:   append([]string(nil), c.context.Accounts...),
		Categories: append([]string(nil), c.context.Categories...),
		History:    append([]HistoryItem(nil), c.context.History...),
	})
	if err != nil {
		return Extraction{}, err
	}
	c.state = next
	return next, nil
}

func (c *Capture) Close(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return nil
	}
	c.closed = true
	return c.extractor.DeleteSession(ctx, c.sessionID)
}
