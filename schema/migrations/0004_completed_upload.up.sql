-- The completed uploads, each keyed by its upload's id: the SHA-256, in
-- lowercase hex, the file is staged under in the input store, its file
-- name, media type, and size, and the time the record expires, indexed so
-- the expired records are found and reclaimed. The job CompleteUpload
-- queued from it and the lowercase hex SHA-256 digest of that request,
-- which a repeat must match, are NULL until the first CompleteUpload
-- records them together. A record past its expiry is missing to
-- CompleteUpload.
CREATE TABLE completed_upload (
    id         text        PRIMARY KEY,
    sha256     text        NOT NULL,
    name       text        NOT NULL,
    media_type text        NOT NULL,
    size       bigint      NOT NULL,
    job_id     text,
    digest     text,
    expires_at timestamptz NOT NULL,
    CONSTRAINT cc_completed_upload_id CHECK (id <> ''),
    CONSTRAINT cc_completed_upload_sha256 CHECK (sha256 <> ''),
    CONSTRAINT cc_completed_upload_size CHECK (size >= 0),
    CONSTRAINT cc_completed_upload_job CHECK ((job_id IS NULL) = (digest IS NULL))
);

CREATE INDEX ix_completed_upload_expires_at ON completed_upload (expires_at);
