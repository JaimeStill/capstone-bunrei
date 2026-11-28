--| tier: standard
-- Inserts one completed upload's record, with no job. Its id is the
-- primary key, so a second completion of the upload, which records the
-- same file, is a unique violation.
INSERT INTO completed_upload (id, sha256, name, media_type, size, expires_at)
VALUES ({{id}}, {{sha256}}, {{name}}, {{media_type}}, {{size:bigint}}, {{expires_at:timestamp with time zone}})
