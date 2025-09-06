-- name: CreateExpense :one
INSERT INTO expenses (day, month, description, amount_cents, primary_category, secondary_category)
VALUES (?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: GetExpensesByMonth :many
SELECT * FROM expenses 
WHERE month = ?
ORDER BY day DESC, created_at DESC;

-- name: GetMonthTotal :one
SELECT CAST(COALESCE(SUM(amount_cents), 0) AS INTEGER) as total
FROM expenses 
WHERE month = ?;

-- name: GetCategorySums :many
SELECT primary_category, CAST(SUM(amount_cents) AS INTEGER) as total_amount
FROM expenses 
WHERE month = ?
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