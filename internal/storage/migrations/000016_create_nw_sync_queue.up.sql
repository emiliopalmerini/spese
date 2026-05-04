CREATE TABLE nw_sync_queue (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    account_id    INTEGER NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    year          INTEGER NOT NULL,
    month         INTEGER NOT NULL,
    amount_cents  INTEGER NOT NULL,
    status        TEXT NOT NULL DEFAULT 'pending'
                   CHECK (status IN ('pending','processing','completed','failed')),
    attempts      INTEGER NOT NULL DEFAULT 0,
    max_attempts  INTEGER NOT NULL DEFAULT 3,
    last_error    TEXT,
    created_at    DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at    DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    processed_at  DATETIME,
    next_retry_at DATETIME,
    UNIQUE (account_id, year, month, status)
);

CREATE INDEX idx_nw_sync_status ON nw_sync_queue(status, next_retry_at);
CREATE INDEX idx_nw_sync_created ON nw_sync_queue(created_at);
