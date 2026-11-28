--| tier: standard
-- The object keys of one upload's committed chunks, in offset order.
SELECT object_key FROM upload_chunk WHERE upload_id = {{upload_id}} ORDER BY chunk_offset
