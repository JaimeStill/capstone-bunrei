--| tier: standard
-- One unexpired completed upload's record: its staged file's SHA-256,
-- name, media type, and size, and the job CompleteUpload queued from it
-- and that request's digest, each NULL until then.
SELECT sha256, name, media_type, size, job_id, digest FROM completed_upload
WHERE id = {{id}} AND expires_at > CURRENT_TIMESTAMP
