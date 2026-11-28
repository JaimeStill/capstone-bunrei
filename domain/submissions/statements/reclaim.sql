--| tier: standard
-- Deletes one submission record only when it has expired, so a Submit can
-- take an expired record's ID.
DELETE FROM submission WHERE id = {{id}} AND expires_at <= CURRENT_TIMESTAMP
