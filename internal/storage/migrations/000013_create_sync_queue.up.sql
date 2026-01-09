-- Sync queue table for SQLite-based sync operations
CREATE TABLE sync_queue (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    operation TEXT NOT NULL CHECK (operation IN ('sync', 'delete')),
    expense_id INTEGER NOT NULL,
    -- For delete operations, store expense data since it's already deleted from DB
    expense_day INTEGER NULL,
    expense_month INTEGER NULL,
    expense_description TEXT NULL,
    expense_amount_cents INTEGER NULL,
    expense_primary TEXT NULL,
    expense_secondary TEXT NULL,
    -- Processing state
    status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'processing', 'completed', 'failed')),
    attempts INTEGER NOT NULL DEFAULT 0,
    max_attempts INTEGER NOT NULL DEFAULT 3,
    last_error TEXT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    processed_at DATETIME NULL,
    next_retry_at DATETIME NULL
);

-- Index for efficient queue polling
CREATE INDEX idx_sync_queue_status_next_retry ON sync_queue(status, next_retry_at);
CREATE INDEX idx_sync_queue_created_at ON sync_queue(created_at);
