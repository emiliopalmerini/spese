-- Replace separate day/month columns with single date column
-- This preserves existing data by converting day/month to full date (using current year 2025)

-- Create new table with date column
CREATE TABLE expenses_new (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    date DATE NOT NULL,
    description TEXT NOT NULL,
    amount_cents INTEGER NOT NULL CHECK (amount_cents > 0),
    primary_category TEXT NOT NULL,
    secondary_category TEXT NOT NULL,
    version INTEGER NOT NULL DEFAULT 1,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    synced_at DATETIME NULL,
    sync_status TEXT DEFAULT 'pending' CHECK (sync_status IN ('pending', 'synced', 'error'))
);

-- Copy data from old table, converting day/month to date
-- Use year 2025 for all expenses
INSERT INTO expenses_new (id, date, description, amount_cents, primary_category, secondary_category, version, created_at, synced_at, sync_status)
SELECT 
    id,
    date('2025-' || printf('%02d', month) || '-' || printf('%02d', day)) as date,
    description,
    amount_cents,
    primary_category,
    secondary_category,
    version,
    created_at,
    synced_at,
    sync_status
FROM expenses;

-- Drop old table and rename new one
DROP TABLE expenses;
ALTER TABLE expenses_new RENAME TO expenses;

-- Recreate indexes
CREATE INDEX idx_expenses_date ON expenses(date);
CREATE INDEX idx_expenses_sync_status ON expenses(sync_status);
CREATE INDEX idx_expenses_created_at ON expenses(created_at);