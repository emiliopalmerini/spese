-- Remove last_execution_date tracking
DROP INDEX IF EXISTS idx_recurrent_expenses_last_execution;
ALTER TABLE recurrent_expenses DROP COLUMN last_execution_date;
