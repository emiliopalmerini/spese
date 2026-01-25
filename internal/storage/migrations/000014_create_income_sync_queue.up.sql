-- Income sync queue table for SQLite-based sync operations
CREATE TABLE income_sync_queue (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    operation TEXT NOT NULL CHECK (operation IN ('sync', 'delete')),
    income_id INTEGER NOT NULL,
    -- For delete operations, store income data since it's already deleted from DB
    income_day INTEGER NULL,
    income_month INTEGER NULL,
    income_description TEXT NULL,
    income_amount_cents INTEGER NULL,
    income_category TEXT NULL,
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
CREATE INDEX idx_income_sync_queue_status_next_retry ON income_sync_queue(status, next_retry_at);
CREATE INDEX idx_income_sync_queue_created_at ON income_sync_queue(created_at);
