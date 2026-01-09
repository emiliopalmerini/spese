CREATE TABLE expenses (
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

CREATE INDEX idx_expenses_date ON expenses(date);
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

-- Recurrent expenses table for managing recurring expenses
CREATE TABLE recurrent_expenses (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    start_date DATE NOT NULL,
    end_date DATE NULL,
    repetition_type TEXT NOT NULL CHECK (repetition_type IN ('daily', 'weekly', 'monthly', 'yearly')),
    description TEXT NOT NULL,
    amount_cents INTEGER NOT NULL CHECK (amount_cents > 0),
    primary_category TEXT NOT NULL,
    secondary_category TEXT NOT NULL,
    is_active BOOLEAN NOT NULL DEFAULT 1,
    last_execution_date DATE NULL,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Create indexes for recurrent expenses
CREATE INDEX idx_recurrent_expenses_active ON recurrent_expenses(is_active);
CREATE INDEX idx_recurrent_expenses_start_date ON recurrent_expenses(start_date);
CREATE INDEX idx_recurrent_expenses_repetition ON recurrent_expenses(repetition_type);
CREATE INDEX idx_recurrent_expenses_last_execution ON recurrent_expenses(last_execution_date);

-- Income categories table
CREATE TABLE income_categories (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL UNIQUE,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Incomes table
CREATE TABLE incomes (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    date DATE NOT NULL,
    description TEXT NOT NULL,
    amount_cents INTEGER NOT NULL CHECK (amount_cents > 0),
    category TEXT NOT NULL,
    version INTEGER NOT NULL DEFAULT 1,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    synced_at DATETIME NULL,
    sync_status TEXT DEFAULT 'pending' CHECK (sync_status IN ('pending', 'synced', 'error'))
);

-- Create indexes for incomes
CREATE INDEX idx_incomes_date ON incomes(date);
CREATE INDEX idx_incomes_category ON incomes(category);
CREATE INDEX idx_incomes_sync_status ON incomes(sync_status);
CREATE INDEX idx_income_categories_name ON income_categories(name);

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