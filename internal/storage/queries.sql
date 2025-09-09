-- name: CreateExpense :one
INSERT INTO expenses (date, description, amount_cents, primary_category, secondary_category)
VALUES (date(?), ?, ?, ?, ?)
RETURNING *;

-- name: GetExpensesByMonth :many
SELECT * FROM expenses 
WHERE strftime('%m', date) = printf('%02d', ?)
ORDER BY date DESC, created_at DESC;

-- name: GetMonthTotal :one
SELECT CAST(COALESCE(SUM(amount_cents), 0) AS INTEGER) as total
FROM expenses 
WHERE strftime('%m', date) = printf('%02d', ?);

-- name: GetCategorySums :many
SELECT primary_category, CAST(SUM(amount_cents) AS INTEGER) as total_amount
FROM expenses 
WHERE strftime('%m', date) = printf('%02d', ?)
GROUP BY primary_category
ORDER BY total_amount DESC;

-- name: GetPendingSyncExpenses :many
SELECT id, version, created_at FROM expenses 
WHERE sync_status = 'pending'
ORDER BY created_at ASC
LIMIT ?;

-- name: MarkExpenseSynced :exec
UPDATE expenses 
SET sync_status = 'synced', synced_at = CURRENT_TIMESTAMP
WHERE id = ?;

-- name: MarkExpenseSyncError :exec
UPDATE expenses 
SET sync_status = 'error'
WHERE id = ?;

-- name: GetExpense :one
SELECT * FROM expenses WHERE id = ?;

-- Primary Categories queries
-- name: GetPrimaryCategories :many
SELECT name FROM primary_categories 
ORDER BY name ASC;

-- name: CreatePrimaryCategory :one
INSERT INTO primary_categories (name)
VALUES (?)
RETURNING id, name, created_at;

-- name: DeletePrimaryCategory :exec
DELETE FROM primary_categories WHERE name = ?;

-- Secondary Categories queries
-- name: GetSecondaryCategories :many
SELECT name FROM secondary_categories 
ORDER BY name ASC;

-- name: GetSecondariesByPrimary :many
SELECT sc.name FROM secondary_categories sc
JOIN primary_categories pc ON sc.primary_category_id = pc.id
WHERE pc.name = ?
ORDER BY sc.name ASC;

-- name: CreateSecondaryCategory :one
INSERT INTO secondary_categories (name, primary_category_id)
VALUES (?, ?)
RETURNING id, name, primary_category_id, created_at;

-- name: DeleteSecondaryCategory :exec
DELETE FROM secondary_categories WHERE name = ?;

-- name: RefreshCategories :exec
DELETE FROM secondary_categories;

-- name: RefreshPrimaryCategories :exec  
DELETE FROM primary_categories;

-- Recurrent Expenses queries
-- name: CreateRecurrentExpense :one
INSERT INTO recurrent_expenses (
    start_date, end_date, repetition_type, description, 
    amount_cents, primary_category, secondary_category
)
VALUES (?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: GetRecurrentExpenses :many
SELECT * FROM recurrent_expenses
WHERE is_active = 1
ORDER BY start_date DESC;

-- name: GetRecurrentExpenseByID :one
SELECT * FROM recurrent_expenses
WHERE id = ?;

-- name: UpdateRecurrentExpense :exec
UPDATE recurrent_expenses
SET start_date = ?, 
    end_date = ?, 
    repetition_type = ?, 
    description = ?,
    amount_cents = ?, 
    primary_category = ?, 
    secondary_category = ?,
    updated_at = CURRENT_TIMESTAMP
WHERE id = ?;

-- name: DeactivateRecurrentExpense :exec
UPDATE recurrent_expenses
SET is_active = 0,
    updated_at = CURRENT_TIMESTAMP
WHERE id = ?;

-- name: DeleteRecurrentExpense :exec
DELETE FROM recurrent_expenses
WHERE id = ?;

-- name: GetActiveRecurrentExpensesByDate :many
SELECT * FROM recurrent_expenses
WHERE is_active = 1
  AND start_date <= ?
  AND (end_date IS NULL OR end_date >= ?)
ORDER BY start_date DESC;