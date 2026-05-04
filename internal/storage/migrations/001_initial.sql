-- Migration 001: initial schema

CREATE TABLE IF NOT EXISTS projects (
    id              TEXT PRIMARY KEY,
    root_path       TEXT NOT NULL UNIQUE,
    name            TEXT NOT NULL,
    schema_version  INTEGER NOT NULL DEFAULT 1,
    embedding_model TEXT NOT NULL DEFAULT '',
    created_at      DATETIME NOT NULL,
    last_indexed_at DATETIME NOT NULL,
    config_json     TEXT NOT NULL DEFAULT '{}'
);

CREATE TABLE IF NOT EXISTS files (
    id            TEXT PRIMARY KEY,
    project_id    TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    relative_path TEXT NOT NULL,
    absolute_path TEXT NOT NULL,
    language      TEXT NOT NULL DEFAULT '',
    content_hash  TEXT NOT NULL DEFAULT '',
    size_bytes    INTEGER NOT NULL DEFAULT 0,
    last_modified DATETIME NOT NULL,
    indexed_at    DATETIME NOT NULL,
    chunk_count   INTEGER NOT NULL DEFAULT 0,
    is_ignored    INTEGER NOT NULL DEFAULT 0,
    UNIQUE(project_id, relative_path)
);

CREATE TABLE IF NOT EXISTS chunks (
    id              TEXT PRIMARY KEY,
    file_id         TEXT NOT NULL REFERENCES files(id) ON DELETE CASCADE,
    project_id      TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    chunk_type      TEXT NOT NULL,
    name            TEXT NOT NULL DEFAULT '',
    content         TEXT NOT NULL,
    start_line      INTEGER NOT NULL,
    end_line        INTEGER NOT NULL,
    parent_chunk_id TEXT NOT NULL DEFAULT '',
    metadata_json   TEXT NOT NULL DEFAULT '{}',
    content_hash    TEXT NOT NULL,
    indexed_at      DATETIME NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_chunks_file_id    ON chunks(file_id);
CREATE INDEX IF NOT EXISTS idx_chunks_project_id ON chunks(project_id);
CREATE INDEX IF NOT EXISTS idx_chunks_chunk_type ON chunks(chunk_type);
CREATE INDEX IF NOT EXISTS idx_chunks_name       ON chunks(name);

CREATE TABLE IF NOT EXISTS symbols (
    id        TEXT PRIMARY KEY,
    chunk_id  TEXT NOT NULL REFERENCES chunks(id) ON DELETE CASCADE,
    file_id   TEXT NOT NULL REFERENCES files(id) ON DELETE CASCADE,
    project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    name      TEXT NOT NULL,
    kind      TEXT NOT NULL,
    line      INTEGER NOT NULL,
    language  TEXT NOT NULL DEFAULT ''
);

CREATE INDEX IF NOT EXISTS idx_symbols_name       ON symbols(name);
CREATE INDEX IF NOT EXISTS idx_symbols_project_id ON symbols(project_id);
CREATE INDEX IF NOT EXISTS idx_symbols_kind       ON symbols(kind);

CREATE TABLE IF NOT EXISTS graph_edges (
    id        TEXT PRIMARY KEY,
    from_id   TEXT NOT NULL,
    to_id     TEXT NOT NULL,
    kind      TEXT NOT NULL,
    file_path TEXT NOT NULL DEFAULT '',
    line      INTEGER NOT NULL DEFAULT 0
);

CREATE INDEX IF NOT EXISTS idx_graph_edges_from ON graph_edges(from_id);
CREATE INDEX IF NOT EXISTS idx_graph_edges_to   ON graph_edges(to_id);

-- Embeddings mapping: chunk_id → rowid in the sqlite-vec virtual table
CREATE TABLE IF NOT EXISTS chunk_embedding_map (
    chunk_id TEXT PRIMARY KEY REFERENCES chunks(id) ON DELETE CASCADE,
    rowid    INTEGER NOT NULL
);

-- Schema version tracking
CREATE TABLE IF NOT EXISTS schema_migrations (
    version    INTEGER PRIMARY KEY,
    applied_at DATETIME NOT NULL
);

INSERT OR IGNORE INTO schema_migrations(version, applied_at) VALUES (1, CURRENT_TIMESTAMP);
