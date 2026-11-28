--| tier: standard
--| returning: completed
-- Records the job CompleteUpload queues from one unexpired completed
-- upload, and the digest of its request, only while the record names no
-- job. Of two CompleteUploads racing on one upload, the second waits on
-- the first's row lock and then finds a job recorded, so it changes
-- nothing and the read returns the first's job and digest.
UPDATE completed_upload SET job_id = {{job_id}}, digest = {{digest}}
WHERE id = {{id}} AND job_id IS NULL AND expires_at > CURRENT_TIMESTAMP
