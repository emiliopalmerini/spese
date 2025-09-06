-- name: CreateExpense :one
INSERT INTO expenses (day, month, description, amount_cents, primary_category, secondary_category)
VALUES (?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: GetExpensesByMonth :many
SELECT * FROM expenses 
WHERE month = ?
ORDER BY day DESC, created_at DESC;

-- name: GetMonthTotal :one
SELECT COALESCE(SUM(amount_cents), 0) as total
FROM expenses 
WHERE month = ?;

-- name: GetCategorySums :many
SELECT primary_category, SUM(amount_cents) as total_amount
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