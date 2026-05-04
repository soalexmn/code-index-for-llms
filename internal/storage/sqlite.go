// Package storage provides SQLite-backed persistence for code-index.
// Vector search uses the sqlite-vec extension; BM25 uses SQLite FTS5.
// Both live in a single .code-index/index.db file - no server required.
package storage

import (
	"database/sql"
	"embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	_ "modernc.org/sqlite"

	"github.com/code-index-for-llms/code-index/pkg/types"
)

//go:embed migrations/*.sql
var migrationFS embed.FS

// SQLiteStore is the production Store implementation.
type SQLiteStore struct {
	db *sql.DB
}

// Open opens (or creates) the SQLite database at the given path.
// Call Migrate() after Open to apply pending schema migrations.
func Open(dbPath string) (*SQLiteStore, error) {
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		return nil, fmt.Errorf("create db directory: %w", err)
	}

	dsn := dbPath + "?_journal=WAL&_foreign_keys=on&_busy_timeout=5000"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	db.SetMaxOpenConns(1) // SQLite writer serialization
	return &SQLiteStore{db: db}, nil
}

func (s *SQLiteStore) Close() error { return s.db.Close() }

// Migrate applies all pending SQL migration files in order.
func (s *SQLiteStore) Migrate() error {
	// Ensure schema_migrations table exists before any migration runs.
	if _, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (
		version    INTEGER PRIMARY KEY,
		applied_at TEXT NOT NULL
	)`); err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}

	entries, err := migrationFS.ReadDir("migrations")
	if err != nil {
		return err
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })

	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		// Extract version number from filename prefix (e.g. "003_..." → 3).
		var version int
		fmt.Sscanf(entry.Name(), "%d", &version)

		// Skip already-applied migrations.
		var count int
		s.db.QueryRow(`SELECT COUNT(*) FROM schema_migrations WHERE version=?`, version).Scan(&count) //nolint:errcheck
		if count > 0 {
			continue
		}

		content, err := migrationFS.ReadFile("migrations/" + entry.Name())
		if err != nil {
			return fmt.Errorf("read migration %s: %w", entry.Name(), err)
		}
		if _, err := s.db.Exec(string(content)); err != nil {
			return fmt.Errorf("apply migration %s: %w", entry.Name(), err)
		}
	}
	return nil
}

// ─── Project ──────────────────────────────────────────────────────────────────

func (s *SQLiteStore) UpsertProject(p types.Project) error {
	langs, _ := json.Marshal(p.Languages)
	_, err := s.db.Exec(`
		INSERT INTO projects(id, root_path, name, schema_version, embedding_model, created_at, last_indexed_at, config_json)
		VALUES (?,?,?,?,?,?,?,?)
		ON CONFLICT(id) DO UPDATE SET
			name=excluded.name,
			embedding_model=excluded.embedding_model,
			last_indexed_at=excluded.last_indexed_at,
			config_json=excluded.config_json`,
		p.ID, p.RootPath, p.Name, p.SchemaVersion, p.EmbeddingModel,
		p.CreatedAt.UTC(), p.LastIndexedAt.UTC(), string(langs),
	)
	return err
}

func (s *SQLiteStore) GetProject(rootPath string) (types.Project, error) {
	row := s.db.QueryRow(`SELECT id, root_path, name, schema_version, embedding_model, created_at, last_indexed_at, config_json FROM projects WHERE root_path=?`, rootPath)
	var p types.Project
	var createdAt, lastIndexedAt string
	var configJSON string
	if err := row.Scan(&p.ID, &p.RootPath, &p.Name, &p.SchemaVersion, &p.EmbeddingModel, &createdAt, &lastIndexedAt, &configJSON); err != nil {
		return p, err
	}
	p.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	p.LastIndexedAt, _ = time.Parse(time.RFC3339, lastIndexedAt)
	_ = json.Unmarshal([]byte(configJSON), &p.Languages)
	return p, nil
}

// ─── File ─────────────────────────────────────────────────────────────────────

func (s *SQLiteStore) UpsertFile(f types.File) error {
	ignored := 0
	if f.IsIgnored {
		ignored = 1
	}
	_, err := s.db.Exec(`
		INSERT INTO files(id, project_id, relative_path, absolute_path, language, content_hash, size_bytes, last_modified, indexed_at, chunk_count, is_ignored)
		VALUES (?,?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT(project_id, relative_path) DO UPDATE SET
			absolute_path=excluded.absolute_path,
			language=excluded.language,
			content_hash=excluded.content_hash,
			size_bytes=excluded.size_bytes,
			last_modified=excluded.last_modified,
			indexed_at=excluded.indexed_at,
			chunk_count=excluded.chunk_count,
			is_ignored=excluded.is_ignored`,
		f.ID, f.ProjectID, f.RelativePath, f.AbsolutePath, f.Language,
		f.ContentHash, f.SizeBytes, f.LastModified.UTC(), f.IndexedAt.UTC(),
		f.ChunkCount, ignored,
	)
	return err
}

func (s *SQLiteStore) GetFile(projectID, relativePath string) (types.File, bool, error) {
	row := s.db.QueryRow(`SELECT id, project_id, relative_path, absolute_path, language, content_hash, size_bytes, last_modified, indexed_at, chunk_count, is_ignored FROM files WHERE project_id=? AND relative_path=?`, projectID, relativePath)
	f, err := scanFile(row)
	if err == sql.ErrNoRows {
		return f, false, nil
	}
	return f, err == nil, err
}

func (s *SQLiteStore) ListFiles(projectID string) ([]types.File, error) {
	rows, err := s.db.Query(`SELECT id, project_id, relative_path, absolute_path, language, content_hash, size_bytes, last_modified, indexed_at, chunk_count, is_ignored FROM files WHERE project_id=?`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var files []types.File
	for rows.Next() {
		f, err := scanFile(rows)
		if err != nil {
			return nil, err
		}
		files = append(files, f)
	}
	return files, rows.Err()
}

func (s *SQLiteStore) DeleteFile(fileID string) error {
	_, err := s.db.Exec(`DELETE FROM files WHERE id=?`, fileID)
	return err
}

// ─── Chunk ────────────────────────────────────────────────────────────────────

func (s *SQLiteStore) UpsertChunks(chunks []types.Chunk) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck

	stmt, err := tx.Prepare(`
		INSERT INTO chunks(id, file_id, project_id, chunk_type, name, content, start_line, end_line, parent_chunk_id, metadata_json, content_hash, indexed_at, file_path, language)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT(id) DO UPDATE SET
			content=excluded.content,
			start_line=excluded.start_line,
			end_line=excluded.end_line,
			metadata_json=excluded.metadata_json,
			content_hash=excluded.content_hash,
			indexed_at=excluded.indexed_at,
			file_path=excluded.file_path,
			language=excluded.language`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, c := range chunks {
		meta, _ := json.Marshal(c.Metadata)
		_, err := stmt.Exec(
			c.ID, c.FileID, c.ProjectID, string(c.ChunkType), c.Name, c.Content,
			c.StartLine, c.EndLine, c.ParentChunkID, string(meta), c.ContentHash, c.IndexedAt.UTC(), c.FilePath, c.Language,
		)
		if err != nil {
			return fmt.Errorf("upsert chunk %s: %w", c.ID, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	// Re-expand FTS name entries with camelCase/snake_case tokens so keyword
	// queries can match individual word components (e.g. "exponential" matches
	// "ExponentialBackoff"). Runs after the trigger-populated entries settle.
	return s.refreshFTSExpansion(chunks)
}

// refreshFTSExpansion rewrites chunk_fts name entries with expanded tokens.
// The SQL INSERT trigger stores the raw chunk name; this replaces it with
// a space-separated string containing the original name plus all camelCase
// and snake_case sub-tokens so BM25 can match partial identifier terms.
func (s *SQLiteStore) refreshFTSExpansion(chunks []types.Chunk) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck

	del, err := tx.Prepare(`DELETE FROM chunk_fts WHERE chunk_id = ?`)
	if err != nil {
		return err
	}
	defer del.Close()

	ins, err := tx.Prepare(`INSERT INTO chunk_fts(chunk_id, name, content) VALUES (?, ?, ?)`)
	if err != nil {
		return err
	}
	defer ins.Close()

	for _, c := range chunks {
		if _, err := del.Exec(c.ID); err != nil {
			return fmt.Errorf("fts delete chunk %s: %w", c.ID, err)
		}
		if _, err := ins.Exec(c.ID, expandTokens(c.Name), c.Content); err != nil {
			return fmt.Errorf("fts insert chunk %s: %w", c.ID, err)
		}
	}
	return tx.Commit()
}

func (s *SQLiteStore) GetChunk(id string) (types.Chunk, error) {
	row := s.db.QueryRow(`SELECT id, file_id, project_id, chunk_type, name, content, start_line, end_line, parent_chunk_id, metadata_json, content_hash, indexed_at, file_path, language FROM chunks WHERE id=?`, id)
	return scanChunk(row)
}

func (s *SQLiteStore) DeleteChunksByFile(fileID string) error {
	_, err := s.db.Exec(`DELETE FROM chunks WHERE file_id=?`, fileID)
	return err
}

// ─── Symbol ───────────────────────────────────────────────────────────────────

func (s *SQLiteStore) UpsertSymbols(symbols []types.Symbol) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck

	stmt, err := tx.Prepare(`
		INSERT OR REPLACE INTO symbols(id, chunk_id, file_id, project_id, name, kind, line, language)
		VALUES (?,?,?,?,?,?,?,?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, sym := range symbols {
		if _, err := stmt.Exec(sym.ID, sym.ChunkID, sym.FileID, "", sym.Name, string(sym.Kind), sym.Line, sym.Language); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *SQLiteStore) DeleteSymbolsByFile(fileID string) error {
	_, err := s.db.Exec(`DELETE FROM symbols WHERE file_id=?`, fileID)
	return err
}

func (s *SQLiteStore) GetFileSymbols(fileID string) ([]types.Symbol, error) {
	rows, err := s.db.Query(
		`SELECT id, chunk_id, file_id, name, kind, line, language FROM symbols WHERE file_id=? ORDER BY line`,
		fileID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanSymbols(rows)
}

func (s *SQLiteStore) FindReferences(projectID, name string) ([]types.Symbol, error) {
	rows, err := s.db.Query(
		`SELECT id, chunk_id, file_id, name, kind, line, language FROM symbols WHERE name=? ORDER BY file_id, line`,
		name)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanSymbols(rows)
}

func scanSymbols(rows *sql.Rows) ([]types.Symbol, error) {
	var syms []types.Symbol
	for rows.Next() {
		var sym types.Symbol
		var kind string
		if err := rows.Scan(&sym.ID, &sym.ChunkID, &sym.FileID, &sym.Name, &kind, &sym.Line, &sym.Language); err != nil {
			return nil, err
		}
		sym.Kind = types.SymbolKind(kind)
		syms = append(syms, sym)
	}
	return syms, rows.Err()
}

// ─── Search ───────────────────────────────────────────────────────────────────

// Search performs hybrid BM25 + (optional) vector search using Reciprocal Rank Fusion.
// Vector search is skipped when req.QueryVector is nil (NoOp embedder path).
func (s *SQLiteStore) Search(req SearchRequest) ([]types.SearchResult, error) {
	if req.Limit <= 0 {
		req.Limit = 10
	}
	candidate := req.Limit * 3 // over-fetch for RRF

	bm25Results, err := s.bm25Search(req, candidate)
	if err != nil {
		return nil, fmt.Errorf("bm25 search: %w", err)
	}

	// No vector - return BM25 results directly.
	if len(req.QueryVector) == 0 {
		if len(bm25Results) > req.Limit {
			bm25Results = bm25Results[:req.Limit]
		}
		return bm25Results, nil
	}

	vectorResults, err := s.vectorSearch(req, candidate)
	if err != nil {
		// Vector search failure is non-fatal; fall back to BM25.
		vectorResults = nil
	}

	fused := rrfFuse(vectorResults, bm25Results, req.Limit)
	return fused, nil
}

// bm25Search uses SQLite FTS5 for keyword matching.
func (s *SQLiteStore) bm25Search(req SearchRequest, limit int) ([]types.SearchResult, error) {
	filter := buildBM25Filter(req)
	args := buildBM25Args(req, limit)

	q := fmt.Sprintf(`
		SELECT c.id, c.file_id, c.project_id, c.chunk_type, c.name, c.content,
		       c.start_line, c.end_line, c.parent_chunk_id, c.metadata_json,
		       c.content_hash, c.indexed_at, c.file_path, c.language,
		       bm25(chunk_fts) as score
		FROM chunk_fts
		JOIN chunks c ON chunk_fts.chunk_id = c.id
		WHERE chunk_fts MATCH ?
		%s
		ORDER BY score
		LIMIT ?`, filter)

	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []types.SearchResult
	for rows.Next() {
		var c types.Chunk
		var ct, metaJSON, idxAt string
		var bm25Score float64
		if err := rows.Scan(
			&c.ID, &c.FileID, &c.ProjectID, &ct, &c.Name, &c.Content,
			&c.StartLine, &c.EndLine, &c.ParentChunkID, &metaJSON,
			&c.ContentHash, &idxAt, &c.FilePath, &c.Language, &bm25Score,
		); err != nil {
			return nil, err
		}
		c.ChunkType = types.ChunkType(ct)
		_ = json.Unmarshal([]byte(metaJSON), &c.Metadata)
		c.IndexedAt, _ = time.Parse(time.RFC3339, idxAt)
		score := -bm25Score // FTS5 returns negative scores; negate for ascending sort
		results = append(results, types.SearchResult{Chunk: c, BM25Score: score, Score: score})
	}
	return results, rows.Err()
}

func buildBM25Filter(req SearchRequest) string {
	var parts []string
	if req.ProjectID != "" {
		parts = append(parts, "c.project_id = ?")
	}
	if req.Language != "" {
		parts = append(parts, "c.language = ?")
	}
	if len(parts) == 0 {
		return ""
	}
	return "AND " + strings.Join(parts, " AND ")
}

func buildBM25Args(req SearchRequest, limit int) []any {
	args := []any{preprocessBM25Query(req.Query)}
	if req.ProjectID != "" {
		args = append(args, req.ProjectID)
	}
	if req.Language != "" {
		args = append(args, req.Language)
	}
	args = append(args, limit)
	return args
}

// preprocessBM25Query converts a free-form query into FTS5 OR syntax.
// Splits on whitespace, expands camelCase and snake_case tokens, deduplicates,
// and joins with OR so that any matching term retrieves the chunk.
func preprocessBM25Query(query string) string {
	seen := map[string]bool{}
	var terms []string

	add := func(t string) {
		t = strings.ToLower(strings.Trim(t, ".,;:!?\"'()[]{}"))
		if len(t) < 2 || seen[t] {
			return
		}
		seen[t] = true
		terms = append(terms, t)
	}

	for _, tok := range strings.Fields(query) {
		add(tok)
		// Expand camelCase: ExponentialBackoff → [Exponential, Backoff]
		for _, sub := range splitCamelCase(tok) {
			add(sub)
		}
		// Expand snake/dot/dash: encode_token → [encode, token]
		for _, sub := range strings.FieldsFunc(tok, func(r rune) bool {
			return r == '_' || r == '.' || r == '-'
		}) {
			add(sub)
			for _, s2 := range splitCamelCase(sub) {
				add(s2)
			}
		}
	}

	if len(terms) == 0 {
		return query
	}
	return strings.Join(terms, " OR ")
}

// splitCamelCase splits a camelCase or PascalCase identifier into words.
// "ExponentialBackoff" → ["Exponential", "Backoff"]
// "encodeToken" → ["encode", "Token"]
func splitCamelCase(s string) []string {
	var words []string
	var word []rune
	for i, r := range s {
		if i > 0 && r >= 'A' && r <= 'Z' {
			if len(word) > 0 {
				words = append(words, string(word))
			}
			word = []rune{r}
		} else {
			word = append(word, r)
		}
	}
	if len(word) > 0 {
		words = append(words, string(word))
	}
	return words
}

// expandTokens returns a space-separated string of the original name plus all
// component tokens from camelCase/snake_case splitting. Used to augment the
// FTS name field so keyword queries can find individual word components.
// Example: "ExponentialBackoff" → "ExponentialBackoff Exponential Backoff"
// Example: "aws_iam_role.lambda_exec" → "aws_iam_role.lambda_exec aws iam role lambda exec"
func expandTokens(name string) string {
	seen := map[string]bool{strings.ToLower(name): true}
	tokens := []string{name}

	add := func(t string) {
		lower := strings.ToLower(t)
		if len(t) < 2 || seen[lower] {
			return
		}
		seen[lower] = true
		tokens = append(tokens, t)
	}

	// Split on separators first
	parts := strings.FieldsFunc(name, func(r rune) bool {
		return r == '_' || r == '.' || r == '-'
	})
	for _, p := range parts {
		add(p)
		for _, sub := range splitCamelCase(p) {
			add(sub)
		}
	}
	// Also split camelCase on the whole name (e.g. "encodeToken")
	for _, sub := range splitCamelCase(name) {
		add(sub)
	}

	return strings.Join(tokens, " ")
}

// vectorSearch queries the sqlite-vec virtual table.
// Returns an error (non-fatal) if sqlite-vec is not loaded.
func (s *SQLiteStore) vectorSearch(_ SearchRequest, _ int) ([]types.SearchResult, error) {
	// sqlite-vec integration: implemented after binary packaging with CGO.
	// For now, return empty so the hybrid path degrades to BM25.
	return nil, nil
}

// rrfFuse merges two ranked lists via Reciprocal Rank Fusion (k=60).
func rrfFuse(vector, bm25 []types.SearchResult, limit int) []types.SearchResult {
	const k = 60.0
	scores := map[string]float64{}
	byID := map[string]types.SearchResult{}

	for rank, r := range vector {
		scores[r.Chunk.ID] += 1.0 / (k + float64(rank+1))
		r.VectorScore = r.Score
		byID[r.Chunk.ID] = r
	}
	for rank, r := range bm25 {
		scores[r.Chunk.ID] += 1.0 / (k + float64(rank+1))
		if existing, ok := byID[r.Chunk.ID]; ok {
			existing.BM25Score = r.BM25Score
			byID[r.Chunk.ID] = existing
		} else {
			r.BM25Score = r.BM25Score
			byID[r.Chunk.ID] = r
		}
	}

	type scored struct {
		id    string
		score float64
	}
	ranked := make([]scored, 0, len(scores))
	for id, sc := range scores {
		ranked = append(ranked, scored{id, sc})
	}
	sort.Slice(ranked, func(i, j int) bool { return ranked[i].score > ranked[j].score })

	if len(ranked) > limit {
		ranked = ranked[:limit]
	}
	results := make([]types.SearchResult, 0, len(ranked))
	for _, r := range ranked {
		res := byID[r.id]
		res.Score = r.score
		results = append(results, res)
	}
	return results
}

// ─── IaC Resource Query ───────────────────────────────────────────────────────

func (s *SQLiteStore) QueryResources(req ResourceQuery) ([]types.Chunk, error) {
	if req.Limit <= 0 {
		req.Limit = 50
	}
	var conditions []string
	var args []any

	conditions = append(conditions, "chunk_type IN ('RESOURCE','MODULE')")

	if req.ProjectID != "" {
		conditions = append(conditions, "project_id = ?")
		args = append(args, req.ProjectID)
	}
	if req.ResourceType != "" {
		conditions = append(conditions, "json_extract(metadata_json, '$.resource_type') = ?")
		args = append(args, req.ResourceType)
	}
	if req.Provider != "" {
		conditions = append(conditions, "json_extract(metadata_json, '$.provider') = ?")
		args = append(args, req.Provider)
	}
	if req.ModulePath != "" {
		conditions = append(conditions, "name LIKE ?")
		args = append(args, req.ModulePath+"%")
	}

	where := "WHERE " + strings.Join(conditions, " AND ")
	args = append(args, req.Limit)

	rows, err := s.db.Query(fmt.Sprintf(`
		SELECT id, file_id, project_id, chunk_type, name, content, start_line, end_line,
		       parent_chunk_id, metadata_json, content_hash, indexed_at, file_path, language
		FROM chunks %s LIMIT ?`, where), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var chunks []types.Chunk
	for rows.Next() {
		c, err := scanChunkRow(rows)
		if err != nil {
			return nil, err
		}
		chunks = append(chunks, c)
	}
	return chunks, rows.Err()
}

// ─── Graph ────────────────────────────────────────────────────────────────────

func (s *SQLiteStore) UpsertGraphEdges(edges []types.GraphEdge) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck

	stmt, err := tx.Prepare(`INSERT OR REPLACE INTO graph_edges(id, from_id, to_id, kind, file_path, line) VALUES (?,?,?,?,?,?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, e := range edges {
		if _, err := stmt.Exec(e.ID, e.FromID, e.ToID, string(e.Kind), e.FilePath, e.Line); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *SQLiteStore) GetDependencyGraph(chunkID string, depth int) ([]types.GraphNode, []types.GraphEdge, error) {
	if depth <= 0 {
		depth = 2
	}

	visited := map[string]bool{chunkID: true}
	frontier := []string{chunkID}
	var allEdges []types.GraphEdge

	for d := 0; d < depth && len(frontier) > 0; d++ {
		placeholders := strings.Repeat("?,", len(frontier))
		placeholders = strings.TrimSuffix(placeholders, ",")
		args := make([]any, len(frontier))
		for i, id := range frontier {
			args[i] = id
		}

		rows, err := s.db.Query(fmt.Sprintf(`SELECT id, from_id, to_id, kind, file_path, line FROM graph_edges WHERE from_id IN (%s) OR to_id IN (%s)`, placeholders, placeholders), append(args, args...)...)
		if err != nil {
			return nil, nil, err
		}

		var nextFrontier []string
		for rows.Next() {
			var e types.GraphEdge
			if err := rows.Scan(&e.ID, &e.FromID, &e.ToID, &e.Kind, &e.FilePath, &e.Line); err != nil {
				rows.Close()
				return nil, nil, err
			}
			allEdges = append(allEdges, e)
			if !visited[e.ToID] {
				visited[e.ToID] = true
				nextFrontier = append(nextFrontier, e.ToID)
			}
			if !visited[e.FromID] {
				visited[e.FromID] = true
				nextFrontier = append(nextFrontier, e.FromID)
			}
		}
		rows.Close()
		frontier = nextFrontier
	}

	// Build nodes from visited chunk IDs.
	var nodes []types.GraphNode
	for id := range visited {
		chunk, err := s.GetChunk(id)
		if err != nil {
			continue
		}
		nodes = append(nodes, types.GraphNode{
			ID:       id,
			ChunkID:  chunk.ID,
			Name:     chunk.Name,
			Kind:     chunk.ChunkType,
			FilePath: chunk.FilePath,
			Language: chunk.Language,
		})
	}
	return nodes, allEdges, nil
}

// ─── Stats ────────────────────────────────────────────────────────────────────

func (s *SQLiteStore) IndexStatus(projectID string) (types.IndexStatus, error) {
	p, err := s.projectByID(projectID)
	if err != nil {
		return types.IndexStatus{}, err
	}

	var totalFiles, totalChunks int
	s.db.QueryRow(`SELECT COUNT(*) FROM files WHERE project_id=?`, projectID).Scan(&totalFiles)   //nolint:errcheck
	s.db.QueryRow(`SELECT COUNT(*) FROM chunks WHERE project_id=?`, projectID).Scan(&totalChunks) //nolint:errcheck

	rows, _ := s.db.Query(`SELECT DISTINCT language FROM files WHERE project_id=? AND language != ''`, projectID)
	var langs []string
	if rows != nil {
		for rows.Next() {
			var lang string
			rows.Scan(&lang) //nolint:errcheck
			langs = append(langs, lang)
		}
		rows.Close()
	}

	stale := totalFiles > 0 && time.Since(p.LastIndexedAt) > 24*time.Hour

	return types.IndexStatus{
		Project:        p,
		TotalFiles:     totalFiles,
		TotalChunks:    totalChunks,
		Languages:      langs,
		LastIndexed:    p.LastIndexedAt,
		IsStale:        stale,
		EmbeddingModel: p.EmbeddingModel,
	}, nil
}

func (s *SQLiteStore) projectByID(id string) (types.Project, error) {
	row := s.db.QueryRow(`SELECT id, root_path, name, schema_version, embedding_model, created_at, last_indexed_at, config_json FROM projects WHERE id=?`, id)
	var p types.Project
	var createdAt, lastIndexedAt, configJSON string
	if err := row.Scan(&p.ID, &p.RootPath, &p.Name, &p.SchemaVersion, &p.EmbeddingModel, &createdAt, &lastIndexedAt, &configJSON); err != nil {
		return p, err
	}
	p.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	p.LastIndexedAt, _ = time.Parse(time.RFC3339, lastIndexedAt)
	_ = json.Unmarshal([]byte(configJSON), &p.Languages)
	return p, nil
}

// ─── Scan helpers ─────────────────────────────────────────────────────────────

type scanner interface {
	Scan(dest ...any) error
}

func scanFile(row scanner) (types.File, error) {
	var f types.File
	var lastMod, indexedAt string
	var ignored int
	err := row.Scan(&f.ID, &f.ProjectID, &f.RelativePath, &f.AbsolutePath, &f.Language,
		&f.ContentHash, &f.SizeBytes, &lastMod, &indexedAt, &f.ChunkCount, &ignored)
	f.LastModified, _ = time.Parse(time.RFC3339, lastMod)
	f.IndexedAt, _ = time.Parse(time.RFC3339, indexedAt)
	f.IsIgnored = ignored != 0
	return f, err
}

func scanChunk(row scanner) (types.Chunk, error) {
	return scanChunkRow(row)
}

func scanChunkRow(row scanner) (types.Chunk, error) {
	var c types.Chunk
	var ct, metaJSON, idxAt string
	err := row.Scan(&c.ID, &c.FileID, &c.ProjectID, &ct, &c.Name, &c.Content,
		&c.StartLine, &c.EndLine, &c.ParentChunkID, &metaJSON, &c.ContentHash, &idxAt, &c.FilePath, &c.Language)
	c.ChunkType = types.ChunkType(ct)
	_ = json.Unmarshal([]byte(metaJSON), &c.Metadata)
	c.IndexedAt, _ = time.Parse(time.RFC3339, idxAt)
	return c, err
}
