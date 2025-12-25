-- Drop indexes
DROP INDEX IF EXISTS idx_incomes_date;
DROP INDEX IF EXISTS idx_incomes_category;
DROP INDEX IF EXISTS idx_incomes_sync_status;
DROP INDEX IF EXISTS idx_income_categories_name;

-- Drop tables
DROP TABLE IF EXISTS incomes;
DROP TABLE IF EXISTS income_categories;
