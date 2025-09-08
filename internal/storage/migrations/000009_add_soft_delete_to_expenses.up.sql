-- Add soft delete support to expenses table
ALTER TABLE expenses ADD COLUMN deleted_at DATETIME NULL;

-- Add index for soft delete queries
CREATE INDEX idx_expenses_deleted_at ON expenses(deleted_at);