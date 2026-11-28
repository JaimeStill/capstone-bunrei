--| tier: standard
-- Inserts one submission record. Its ID is the primary key, so an ID
-- another Submit holds is a unique violation, which a concurrent insert
-- of the same ID meets once the first commits.
INSERT INTO submission (id, job_id, digest, expires_at)
VALUES ({{id}}, {{job_id}}, {{digest}}, {{expires_at:timestamp with time zone}})
