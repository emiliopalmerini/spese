-- Revert date column back to separate day/month columns

-- Create new table with day/month columns
CREATE TABLE expenses_new (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    day INTEGER NOT NULL CHECK (day >= 1 AND day <= 31),
    month INTEGER NOT NULL CHECK (month >= 1 AND month <= 12),
    description TEXT NOT NULL,
    amount_cents INTEGER NOT NULL CHECK (amount_cents > 0),
    primary_category TEXT NOT NULL,
    secondary_category TEXT NOT NULL,
    version INTEGER NOT NULL DEFAULT 1,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    synced_at DATETIME NULL,
    sync_status TEXT DEFAULT 'pending' CHECK (sync_status IN ('pending', 'synced', 'error'))
);

-- Copy data back, extracting day/month from date
INSERT INTO expenses_new (id, day, month, description, amount_cents, primary_category, secondary_category, version, created_at, synced_at, sync_status)
SELECT 
    id,
    CAST(strftime('%d', date) AS INTEGER) as day,
    CAST(strftime('%m', date) AS INTEGER) as month,
    description,
    amount_cents,
    primary_category,
    secondary_category,
    version,
    created_at,
    synced_at,
    sync_status
FROM expenses;

-- Drop new table and rename back
DROP TABLE expenses;
ALTER TABLE expenses_new RENAME TO expenses;

-- Recreate old indexes
CREATE INDEX idx_expenses_month ON expenses(month, day);
CREATE INDEX idx_expenses_sync_status ON expenses(sync_status);
CREATE INDEX idx_expenses_created_at ON expenses(created_at);