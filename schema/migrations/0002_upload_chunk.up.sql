-- The committed chunks of each upload in progress, keyed by the upload
-- and the offset each starts at, so an upload's chunks read back in offset
-- order. A chunk's bytes are the object at object_key in blob storage,
-- <upload prefix><upload>/<offset>/<append id>: the key is unique per
-- append, so the row, not the object, is what commits a chunk, and an
-- object no row names never reads back. A chunk row lives only
-- as long as its upload: deleting or reclaiming the upload removes its
-- rows, and the objects are left to the upload prefix's expiry.
CREATE TABLE upload_chunk (
    upload_id    text   NOT NULL,
    chunk_offset bigint NOT NULL,
    object_key   text   NOT NULL,
    CONSTRAINT pk_upload_chunk PRIMARY KEY (upload_id, chunk_offset),
    CONSTRAINT fk_upload_chunk_upload FOREIGN KEY (upload_id) REFERENCES upload (id) ON DELETE CASCADE,
    CONSTRAINT cc_upload_chunk_offset CHECK (chunk_offset >= 0),
    CONSTRAINT cc_upload_chunk_object_key CHECK (object_key <> '')
);
