package sheets

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// FileClient is a local sheet-mirror target for development and smoke tests.
// It preserves the worker/outbox path while avoiding Google Sheets writes.
type FileClient struct {
	path string
	mu   sync.Mutex
}

// NewFileClient returns a file-backed sheet target. The file is JSON shaped as
// {"accounts":[...],"transactions":[...],"snapshots":[...]}.
func NewFileClient(path string) (*FileClient, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, errors.New("local sheet path is required")
	}
	return &FileClient{path: path}, nil
}

// ReplaceRows replaces one tab in the local JSON file.
func (c *FileClient) ReplaceRows(ctx context.Context, tab string, rows [][]any) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	data, err := c.read()
	if err != nil {
		return err
	}
	data[tab] = rows
	return c.write(ctx, data)
}

func (c *FileClient) read() (map[string][][]any, error) {
	raw, err := os.ReadFile(c.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return make(map[string][][]any), nil
		}
		return nil, fmt.Errorf("read local sheet %s: %w", c.path, err)
	}
	if len(strings.TrimSpace(string(raw))) == 0 {
		return make(map[string][][]any), nil
	}

	var data map[string][][]any
	if err := json.Unmarshal(raw, &data); err != nil {
		return nil, fmt.Errorf("parse local sheet %s: %w", c.path, err)
	}
	if data == nil {
		data = make(map[string][][]any)
	}
	return data, nil
}

func (c *FileClient) write(ctx context.Context, data map[string][][]any) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	dir := filepath.Dir(c.path)
	if dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("create local sheet dir %s: %w", dir, err)
		}
	}

	raw, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("encode local sheet: %w", err)
	}
	raw = append(raw, '\n')

	tmp, err := os.CreateTemp(dir, ".local-sheet-*.json")
	if err != nil {
		return fmt.Errorf("create local sheet temp file: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)

	if _, err := tmp.Write(raw); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write local sheet temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close local sheet temp file: %w", err)
	}
	if err := os.Rename(tmpName, c.path); err != nil {
		return fmt.Errorf("replace local sheet %s: %w", c.path, err)
	}
	return nil
}
