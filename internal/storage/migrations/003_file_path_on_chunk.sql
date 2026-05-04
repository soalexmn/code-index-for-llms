-- Migration 003: add file_path column to chunks for direct retrieval without JOIN

ALTER TABLE chunks ADD COLUMN file_path TEXT NOT NULL DEFAULT '';

-- Backfill from files table
UPDATE chunks SET file_path = (
    SELECT relative_path FROM files WHERE files.id = chunks.file_id
);

INSERT OR IGNORE INTO schema_migrations(version, applied_at) VALUES (3, CURRENT_TIMESTAMP);
