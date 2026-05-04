package services

import (
	"context"
	"errors"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"spese/internal/core"
	"spese/internal/storage"
)

type fakeNetWorthWriter struct {
	calls atomic.Int64
	err   error
	last  struct {
		name      string
		accType   core.AccountType
		year, mon int
		cents     int64
	}
}

func (f *fakeNetWorthWriter) UpsertBalance(ctx context.Context, name string, t core.AccountType,
	year, month int, amount core.Money) (string, error) {
	f.calls.Add(1)
	f.last.name = name
	f.last.accType = t
	f.last.year = year
	f.last.mon = month
	f.last.cents = amount.Cents
	if f.err != nil {
		return "", f.err
	}
	return "ref:" + name, nil
}

func newNwTestRepo(t *testing.T) *storage.SQLiteRepository {
	t.Helper()
	dir := t.TempDir()
	repo, err := storage.NewSQLiteRepository(filepath.Join(dir, "nw.db"))
	if err != nil {
		t.Fatalf("repo: %v", err)
	}
	t.Cleanup(func() { _ = repo.Close() })
	return repo
}

func seedAccountAndQueue(t *testing.T, repo *storage.SQLiteRepository, name string) int64 {
	t.Helper()
	id, err := repo.CreateAccount(context.Background(), core.Account{Name: name, Type: core.AccountCash, Active: true})
	if err != nil {
		t.Fatalf("create account: %v", err)
	}
	if err := repo.UpsertBalanceAndEnqueueSync(context.Background(), core.AccountBalance{
		AccountID: id, Year: 2025, Month: 6, Amount: core.Money{Cents: 12345},
	}); err != nil {
		t.Fatalf("upsert+enqueue: %v", err)
	}
	return id
}

func TestNetWorthProcessor_ProcessSuccess(t *testing.T) {
	repo := newNwTestRepo(t)
	seedAccountAndQueue(t, repo, "BCC")
	writer := &fakeNetWorthWriter{}

	p := NewNetWorthSyncProcessor(repo, writer, SyncProcessorConfig{
		BatchSize:  10,
		MaxRetries: 3,
	})
	p.ProcessOnce(context.Background())

	if writer.calls.Load() != 1 {
		t.Fatalf("expected 1 writer call, got %d", writer.calls.Load())
	}
	if writer.last.name != "BCC" || writer.last.year != 2025 || writer.last.mon != 6 || writer.last.cents != 12345 {
		t.Fatalf("unexpected last call: %+v", writer.last)
	}

	stats, err := repo.GetNetWorthSyncQueueStats(context.Background())
	if err != nil {
		t.Fatalf("stats: %v", err)
	}
	if stats.CompletedCount != 1 || stats.PendingCount != 0 {
		t.Fatalf("expected 1 completed/0 pending, got %+v", stats)
	}
}

func TestNetWorthProcessor_RetryOnError(t *testing.T) {
	repo := newNwTestRepo(t)
	seedAccountAndQueue(t, repo, "BCC")
	writer := &fakeNetWorthWriter{err: errors.New("sheet quota")}

	p := NewNetWorthSyncProcessor(repo, writer, SyncProcessorConfig{
		BatchSize:  10,
		MaxRetries: 3,
	})
	p.ProcessOnce(context.Background())

	stats, err := repo.GetNetWorthSyncQueueStats(context.Background())
	if err != nil {
		t.Fatalf("stats: %v", err)
	}
	// First failure: still pending (with backoff), attempts=1
	if stats.PendingCount != 1 || stats.FailedCount != 0 {
		t.Fatalf("expected pending=1 failed=0, got %+v", stats)
	}
}

func TestNetWorthProcessor_MaxAttemptsMarkFailed(t *testing.T) {
	repo := newNwTestRepo(t)
	seedAccountAndQueue(t, repo, "BCC")
	writer := &fakeNetWorthWriter{err: errors.New("perma fail")}

	cfg := SyncProcessorConfig{BatchSize: 10, MaxRetries: 1}
	p := NewNetWorthSyncProcessor(repo, writer, cfg)

	// First attempt: writer fails, attempts will become 1 → equals MaxRetries → marked failed.
	p.ProcessOnce(context.Background())

	stats, err := repo.GetNetWorthSyncQueueStats(context.Background())
	if err != nil {
		t.Fatalf("stats: %v", err)
	}
	if stats.FailedCount != 1 {
		t.Fatalf("expected failed=1, got %+v", stats)
	}
}

func TestNetWorthProcessor_StartStop(t *testing.T) {
	repo := newNwTestRepo(t)
	writer := &fakeNetWorthWriter{}
	p := NewNetWorthSyncProcessor(repo, writer, SyncProcessorConfig{
		PollInterval:    10 * time.Millisecond,
		CleanupInterval: time.Hour,
		BatchSize:       10,
		MaxRetries:      3,
	})
	if err := p.Start(context.Background()); err != nil {
		t.Fatalf("start: %v", err)
	}
	if !p.IsRunning() {
		t.Fatal("expected running")
	}
	if err := p.Start(context.Background()); err == nil {
		t.Fatal("expected error on double start")
	}
	if err := p.Stop(context.Background()); err != nil {
		t.Fatalf("stop: %v", err)
	}
	if p.IsRunning() {
		t.Fatal("expected stopped")
	}
}
