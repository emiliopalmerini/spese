-- Create income categories table
CREATE TABLE income_categories (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL UNIQUE,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Create incomes table
CREATE TABLE incomes (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    date DATE NOT NULL,
    description TEXT NOT NULL,
    amount_cents INTEGER NOT NULL,
    category TEXT NOT NULL,
    version INTEGER DEFAULT 1,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    synced_at DATETIME,
    sync_status TEXT DEFAULT 'pending'
);

-- Create indexes for better performance
CREATE INDEX idx_incomes_date ON incomes(date);
CREATE INDEX idx_incomes_category ON incomes(category);
CREATE INDEX idx_incomes_sync_status ON incomes(sync_status);
CREATE INDEX idx_income_categories_name ON income_categories(name);

-- Insert income categories
INSERT INTO income_categories (name) VALUES
('Stipendio E'),
('Stipendio G'),
('Freelance E'),
('Freelance G'),
('2DM'),
('Interessi'),
('Regali'),
('Rimborsi');
