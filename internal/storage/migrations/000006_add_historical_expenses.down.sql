-- Remove historical expenses (rollback migration)
-- Delete all expenses with sync_status = 'synced' that were added by this migration

DELETE FROM expenses WHERE sync_status = 'synced' AND created_at >= '2025-09-06';