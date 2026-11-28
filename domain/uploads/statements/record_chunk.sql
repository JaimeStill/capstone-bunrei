--| tier: standard
--| transaction: required
-- Records one chunk of an upload at the offset it starts at, naming the
-- object that holds its bytes, in the transaction that advanced the
-- upload past it.
INSERT INTO upload_chunk (upload_id, chunk_offset, object_key)
VALUES ({{upload_id}}, {{chunk_offset:bigint}}, {{object_key}})
