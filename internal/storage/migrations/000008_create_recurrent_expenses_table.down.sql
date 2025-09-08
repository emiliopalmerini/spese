-- Drop recurrent_expenses table and related objects
DROP TRIGGER IF EXISTS update_recurrent_expenses_updated_at;
DROP INDEX IF EXISTS idx_recurrent_expenses_repetition;
DROP INDEX IF EXISTS idx_recurrent_expenses_start_date;
DROP INDEX IF EXISTS idx_recurrent_expenses_active;
DROP TABLE IF EXISTS recurrent_expenses;