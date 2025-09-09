-- Add back soft delete functionality to expenses table
ALTER TABLE expenses ADD COLUMN deleted_at DATETIME NULL;

-- Create index for performance optimization on deleted_at column
CREATE INDEX idx_expenses_deleted_at ON expenses(deleted_at);