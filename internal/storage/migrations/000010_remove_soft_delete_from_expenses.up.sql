-- Remove soft delete functionality from expenses table
DROP INDEX IF EXISTS idx_expenses_deleted_at;
ALTER TABLE expenses DROP COLUMN deleted_at;