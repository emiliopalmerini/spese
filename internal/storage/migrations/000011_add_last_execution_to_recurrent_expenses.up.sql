-- Add last_execution_date to track when recurring expense was last processed
ALTER TABLE recurrent_expenses ADD COLUMN last_execution_date DATE NULL;

-- Create index for efficient querying of due recurring expenses
CREATE INDEX idx_recurrent_expenses_last_execution ON recurrent_expenses(last_execution_date);
