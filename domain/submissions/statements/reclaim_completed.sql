--| tier: standard
-- Deletes at most budget expired completed upload records, earliest
-- expiry first: the expired records' reclaimer's pass. The subquery reads
-- ix_completed_upload_expires_at in expiry order and stops at the budget.
DELETE FROM completed_upload
WHERE id IN (
    SELECT id FROM completed_upload
    WHERE expires_at <= CURRENT_TIMESTAMP
    ORDER BY expires_at
    FETCH FIRST {{budget:bigint}} ROWS ONLY
)
