--| tier: standard
-- One unexpired submission record: the job it names and its request's
-- digest.
SELECT job_id, digest FROM submission WHERE id = {{id}} AND expires_at > CURRENT_TIMESTAMP
