--| tier: standard
-- Deletes at most budget expired submission records, earliest expiry
-- first: the expired records' reclaimer's pass. The subquery reads
-- ix_submission_expires_at in expiry order and stops at the budget.
DELETE FROM submission
WHERE id IN (
    SELECT id FROM submission
    WHERE expires_at <= CURRENT_TIMESTAMP
    ORDER BY expires_at
    FETCH FIRST {{budget:bigint}} ROWS ONLY
)
