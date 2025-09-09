-- Create recurrent_expenses table for managing recurring expenses
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
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Create indexes for better query performance
CREATE INDEX idx_recurrent_expenses_active ON recurrent_expenses(is_active);
CREATE INDEX idx_recurrent_expenses_start_date ON recurrent_expenses(start_date);
CREATE INDEX idx_recurrent_expenses_repetition ON recurrent_expenses(repetition_type);

-- Create trigger to update updated_at on row update
CREATE TRIGGER update_recurrent_expenses_updated_at 
AFTER UPDATE ON recurrent_expenses
BEGIN
    UPDATE recurrent_expenses SET updated_at = CURRENT_TIMESTAMP WHERE id = NEW.id;
END;