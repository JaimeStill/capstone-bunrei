-- The idempotency records of Submit, each keyed by the caller's ID: the
-- job the first Submit under the ID minted, the lowercase hex SHA-256
-- digest of that Submit's request, which a retry must match, and the time
-- the record expires, indexed so the expired records are found and
-- reclaimed. A record past its expiry no longer holds its ID: a Submit
-- under the ID reclaims it and records a new job.
CREATE TABLE submission (
    id         text        PRIMARY KEY,
    job_id     text        NOT NULL,
    digest     text        NOT NULL,
    expires_at timestamptz NOT NULL,
    CONSTRAINT cc_submission_id CHECK (id <> ''),
    CONSTRAINT cc_submission_job_id CHECK (job_id <> ''),
    CONSTRAINT cc_submission_digest CHECK (digest <> '')
);

CREATE INDEX ix_submission_expires_at ON submission (expires_at);
