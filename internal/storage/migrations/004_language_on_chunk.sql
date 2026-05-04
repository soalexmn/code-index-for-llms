-- Migration 004: add language column to chunks for language-filtered search

ALTER TABLE chunks ADD COLUMN language TEXT NOT NULL DEFAULT '';

-- Backfill from files table
UPDATE chunks SET language = (
    SELECT language FROM files WHERE files.id = chunks.file_id
);

-- Rebuild FTS to include the updated rows (triggers will keep it in sync going forward)
DELETE FROM chunk_fts;
INSERT INTO chunk_fts(chunk_id, name, content)
    SELECT id, name, content FROM chunks;

INSERT OR IGNORE INTO schema_migrations(version, applied_at) VALUES (4, CURRENT_TIMESTAMP);
