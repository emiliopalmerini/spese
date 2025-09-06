CREATE TABLE expenses (
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

CREATE INDEX idx_expenses_month ON expenses(month, day);
CREATE INDEX idx_expenses_sync_status ON expenses(sync_status);
CREATE INDEX idx_expenses_created_at ON expenses(created_at);

-- Primary categories table
CREATE TABLE primary_categories (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL UNIQUE,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Secondary categories table with foreign key to primary
CREATE TABLE secondary_categories (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    primary_category_id INTEGER NOT NULL,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (primary_category_id) REFERENCES primary_categories(id) ON DELETE CASCADE
);

-- Create indexes for better performance
CREATE INDEX idx_secondary_categories_primary_id ON secondary_categories(primary_category_id);
CREATE INDEX idx_primary_categories_name ON primary_categories(name);
CREATE INDEX idx_secondary_categories_name ON secondary_categories(name);