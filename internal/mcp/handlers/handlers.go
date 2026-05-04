// Package handlers implements the MCP tool handlers.
// Each exported function matches the ToolHandler signature:
//   func(params json.RawMessage) (any, error)
package handlers

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"time"

	"github.com/code-index-for-llms/code-index/internal/config"
	"github.com/code-index-for-llms/code-index/internal/indexer"
	"github.com/code-index-for-llms/code-index/internal/parser"
	"github.com/code-index-for-llms/code-index/internal/parser/generic"
	golang "github.com/code-index-for-llms/code-index/internal/parser/golang"
	"github.com/code-index-for-llms/code-index/internal/parser/python"
	"github.com/code-index-for-llms/code-index/internal/parser/terraform"
	"github.com/code-index-for-llms/code-index/internal/parser/typescript"
	"github.com/code-index-for-llms/code-index/internal/storage"
	"github.com/code-index-for-llms/code-index/pkg/types"
)

// Handlers holds dependencies shared across all MCP tool handlers.
type Handlers struct {
	defaultRoot string // Workspace root when not specified in params
}

func New(defaultRoot string) *Handlers {
	return &Handlers{defaultRoot: defaultRoot}
}

// ─── index_project ────────────────────────────────────────────────────────────

func (h *Handlers) IndexProject(params json.RawMessage) (any, error) {
	var p struct {
		RootPath string `json:"root_path"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, err
	}
	root := resolveRoot(coalesce(p.RootPath, h.defaultRoot))
	if root == "" {
		return nil, fmt.Errorf("root_path required")
	}
	return RunIndexProject(root)
}

// RunIndexProject performs a full index of root. Safe to call from goroutines.
func RunIndexProject(root string) (map[string]any, error) {
	cfg, err := config.Load(root)
	if err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}

	dbPath := filepath.Join(root, cfg.Storage.Path)
	store, err := storage.Open(dbPath)
	if err != nil {
		return nil, fmt.Errorf("open store: %w", err)
	}
	defer store.Close()

	if err := store.Migrate(); err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}

	reg := BuildRegistry()
	chunker := indexer.NewChunker(reg)

	projectID := indexer.ProjectID(root)
	projectName := cfg.Project.Name
	if projectName == "" {
		projectName = filepath.Base(root)
	}

	project := types.Project{
		ID:             projectID,
		RootPath:       root,
		Name:           projectName,
		CreatedAt:      time.Now(),
		LastIndexedAt:  time.Now(),
		SchemaVersion:  1,
		EmbeddingModel: cfg.Embedding.Model,
	}
	if err := store.UpsertProject(project); err != nil {
		return nil, err
	}

	start := time.Now()
	files, err := indexer.Walk(root, cfg)
	if err != nil {
		return nil, fmt.Errorf("walk: %w", err)
	}

	langSet := map[string]bool{}
	var totalChunks int

	for _, fe := range files {
		lang := chunker.DetectLanguage(fe.RelPath, fe.Content)
		fe.Language = lang
		if lang != "" && lang != "generic" {
			langSet[lang] = true
		}

		fileID := indexer.FileID(projectID, fe.RelPath)
		f := types.File{
			ID:           fileID,
			ProjectID:    projectID,
			RelativePath: fe.RelPath,
			AbsolutePath: fe.AbsPath,
			Language:     lang,
			ContentHash:  fe.ContentHash,
			SizeBytes:    fe.SizeBytes,
			LastModified: time.Now(),
			IndexedAt:    time.Now(),
		}

		chunks, symbols, err := chunker.ChunkFile(fileID, projectID, fe)
		if err != nil {
			continue // Skip bad files; don't abort entire index.
		}

		// Ensure ProjectID and FilePath are set on all chunks.
		for i := range chunks {
			chunks[i].ProjectID = projectID
			if chunks[i].FilePath == "" {
				chunks[i].FilePath = fe.RelPath
			}
		}

		f.ChunkCount = len(chunks)
		totalChunks += len(chunks)

		if err := store.UpsertFile(f); err != nil {
			continue
		}
		if err := store.UpsertChunks(chunks); err != nil {
			continue
		}
		if len(symbols) > 0 {
			_ = store.UpsertSymbols(symbols) // Non-fatal.
		}
	}

	langs := make([]string, 0, len(langSet))
	for l := range langSet {
		langs = append(langs, l)
	}
	project.Languages = langs
	project.TotalFiles = len(files)
	project.TotalChunks = totalChunks
	_ = store.UpsertProject(project)

	return map[string]any{
		"project_id":         projectID,
		"files_indexed":      len(files),
		"chunks_created":     totalChunks,
		"languages_detected": langs,
		"duration_ms":        time.Since(start).Milliseconds(),
	}, nil
}

// ─── refresh_index ────────────────────────────────────────────────────────────

func (h *Handlers) RefreshIndex(params json.RawMessage) (any, error) {
	var p struct {
		RootPath     string   `json:"root_path"`
		ChangedFiles []string `json:"changed_files"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, err
	}
	root := resolveRoot(coalesce(p.RootPath, h.defaultRoot))
	if root == "" {
		return nil, fmt.Errorf("root_path required")
	}
	return RunRefresh(root)
}

// RunRefresh performs an incremental index refresh of root. Safe to call from goroutines.
func RunRefresh(root string) (map[string]any, error) {
	cfg, _ := config.Load(root)
	dbPath := filepath.Join(root, cfg.Storage.Path)
	store, err := storage.Open(dbPath)
	if err != nil {
		return nil, err
	}
	defer store.Close()
	_ = store.Migrate()

	project, err := store.GetProject(root)
	if err != nil {
		return nil, fmt.Errorf("project not indexed yet; run index_project first")
	}

	reg := BuildRegistry()
	chunker := indexer.NewChunker(reg)

	currentFiles, _ := indexer.Walk(root, cfg)
	existingFiles, _ := store.ListFiles(project.ID)

	existingByPath := map[string]types.File{}
	for _, f := range existingFiles {
		existingByPath[f.RelativePath] = f
	}

	var updated, removed int

	// Detect changed and new files.
	for _, fe := range currentFiles {
		existing, found := existingByPath[fe.RelPath]
		if found && existing.ContentHash == fe.ContentHash {
			continue // Unchanged.
		}

		fileID := indexer.FileID(project.ID, fe.RelPath)
		if found {
			_ = store.DeleteChunksByFile(existing.ID)
			_ = store.DeleteSymbolsByFile(existing.ID)
		}

		chunks, symbols, err := chunker.ChunkFile(fileID, project.ID, fe)
		if err != nil {
			continue
		}
		for i := range chunks {
			chunks[i].ProjectID = project.ID
		}
		f := types.File{
			ID:           fileID,
			ProjectID:    project.ID,
			RelativePath: fe.RelPath,
			AbsolutePath: fe.AbsPath,
			Language:     chunker.DetectLanguage(fe.RelPath, fe.Content),
			ContentHash:  fe.ContentHash,
			SizeBytes:    fe.SizeBytes,
			LastModified: time.Now(),
			IndexedAt:    time.Now(),
			ChunkCount:   len(chunks),
		}
		_ = store.UpsertFile(f)
		_ = store.UpsertChunks(chunks)
		_ = store.UpsertSymbols(symbols)
		updated++
	}

	// Detect removed files.
	currentByPath := map[string]bool{}
	for _, fe := range currentFiles {
		currentByPath[fe.RelPath] = true
	}
	for _, f := range existingFiles {
		if !currentByPath[f.RelativePath] {
			_ = store.DeleteFile(f.ID)
			removed++
		}
	}

	return map[string]any{
		"files_updated": updated,
		"files_removed": removed,
	}, nil
}

// ─── get_index_status ─────────────────────────────────────────────────────────

func (h *Handlers) GetIndexStatus(params json.RawMessage) (any, error) {
	var p struct {
		RootPath string `json:"root_path"`
	}
	_ = json.Unmarshal(params, &p)
	root := resolveRoot(coalesce(p.RootPath, h.defaultRoot))

	cfg, _ := config.Load(root)
	dbPath := filepath.Join(root, cfg.Storage.Path)
	store, err := storage.Open(dbPath)
	if err != nil {
		return map[string]any{"indexed": false, "error": err.Error()}, nil
	}
	defer store.Close()

	project, err := store.GetProject(root)
	if err != nil {
		return map[string]any{"indexed": false}, nil
	}

	status, err := store.IndexStatus(project.ID)
	if err != nil {
		return nil, err
	}
	return status, nil
}

// ─── search_code ──────────────────────────────────────────────────────────────

func (h *Handlers) SearchCode(params json.RawMessage) (any, error) {
	var p struct {
		Query        string   `json:"query"`
		Language     string   `json:"language"`
		ChunkTypes   []string `json:"chunk_types"`
		Limit        int      `json:"limit"`
		HybridWeight float64  `json:"hybrid_weight"`
		RootPath     string   `json:"root_path"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, err
	}
	if p.Query == "" {
		return nil, fmt.Errorf("query required")
	}
	if p.Limit <= 0 {
		p.Limit = 10
	}

	root := resolveRoot(coalesce(p.RootPath, h.defaultRoot))
	cfg, _ := config.Load(root)
	store, err := storage.Open(filepath.Join(root, cfg.Storage.Path))
	if err != nil {
		return nil, err
	}
	defer store.Close()

	project, err := store.GetProject(root)
	if err != nil {
		return nil, fmt.Errorf("project not indexed")
	}

	chunkTypes := make([]types.ChunkType, len(p.ChunkTypes))
	for i, ct := range p.ChunkTypes {
		chunkTypes[i] = types.ChunkType(ct)
	}

	results, err := store.Search(storage.SearchRequest{
		ProjectID:    project.ID,
		Query:        p.Query,
		Language:     p.Language,
		ChunkTypes:   chunkTypes,
		Limit:        p.Limit,
		HybridWeight: p.HybridWeight,
	})
	if err != nil {
		return nil, err
	}
	return map[string]any{"results": results, "count": len(results)}, nil
}

// ─── query_resources ──────────────────────────────────────────────────────────

func (h *Handlers) QueryResources(params json.RawMessage) (any, error) {
	var p struct {
		ResourceType string `json:"resource_type"`
		Provider     string `json:"provider"`
		ModulePath   string `json:"module_path"`
		Limit        int    `json:"limit"`
		RootPath     string `json:"root_path"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, err
	}

	root := resolveRoot(coalesce(p.RootPath, h.defaultRoot))
	cfg, _ := config.Load(root)
	store, err := storage.Open(filepath.Join(root, cfg.Storage.Path))
	if err != nil {
		return nil, err
	}
	defer store.Close()

	project, err := store.GetProject(root)
	if err != nil {
		return nil, fmt.Errorf("project not indexed")
	}

	chunks, err := store.QueryResources(storage.ResourceQuery{
		ProjectID:    project.ID,
		ResourceType: p.ResourceType,
		Provider:     p.Provider,
		ModulePath:   p.ModulePath,
		Limit:        p.Limit,
	})
	if err != nil {
		return nil, err
	}
	return map[string]any{"resources": chunks, "count": len(chunks)}, nil
}

// ─── list_languages ───────────────────────────────────────────────────────────

func (h *Handlers) ListLanguages(params json.RawMessage) (any, error) {
	reg := BuildRegistry()
	return map[string]any{
		"parsers_available": reg.ListLanguages(),
	}, nil
}

// ─── get_chunk ────────────────────────────────────────────────────────────────

func (h *Handlers) GetChunk(params json.RawMessage) (any, error) {
	var p struct {
		ChunkID      string `json:"chunk_id"`
		ContextLines int    `json:"context_lines"`
		RootPath     string `json:"root_path"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, err
	}
	if p.ChunkID == "" {
		return nil, fmt.Errorf("chunk_id required")
	}

	root := resolveRoot(coalesce(p.RootPath, h.defaultRoot))
	cfg, _ := config.Load(root)
	store, err := storage.Open(filepath.Join(root, cfg.Storage.Path))
	if err != nil {
		return nil, err
	}
	defer store.Close()

	chunk, err := store.GetChunk(p.ChunkID)
	if err != nil {
		return nil, fmt.Errorf("chunk not found: %s", p.ChunkID)
	}
	return map[string]any{"chunk": chunk}, nil
}

// ─── get_file_symbols ─────────────────────────────────────────────────────────

func (h *Handlers) GetFileSymbols(params json.RawMessage) (any, error) {
	var p struct {
		FilePath string `json:"file_path"`
		RootPath string `json:"root_path"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, err
	}
	if p.FilePath == "" {
		return nil, fmt.Errorf("file_path required")
	}

	root := resolveRoot(coalesce(p.RootPath, h.defaultRoot))
	cfg, _ := config.Load(root)
	store, err := storage.Open(filepath.Join(root, cfg.Storage.Path))
	if err != nil {
		return nil, err
	}
	defer store.Close()

	project, err := store.GetProject(root)
	if err != nil {
		return nil, fmt.Errorf("project not indexed")
	}

	fileID := indexer.FileID(project.ID, p.FilePath)
	syms, err := store.GetFileSymbols(fileID)
	if err != nil {
		return nil, err
	}
	return map[string]any{"symbols": syms, "count": len(syms)}, nil
}

// ─── find_references ──────────────────────────────────────────────────────────

func (h *Handlers) FindReferences(params json.RawMessage) (any, error) {
	var p struct {
		Name     string `json:"name"`
		RootPath string `json:"root_path"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, err
	}
	if p.Name == "" {
		return nil, fmt.Errorf("name required")
	}

	root := resolveRoot(coalesce(p.RootPath, h.defaultRoot))
	cfg, _ := config.Load(root)
	store, err := storage.Open(filepath.Join(root, cfg.Storage.Path))
	if err != nil {
		return nil, err
	}
	defer store.Close()

	project, err := store.GetProject(root)
	if err != nil {
		return nil, fmt.Errorf("project not indexed")
	}

	syms, err := store.FindReferences(project.ID, p.Name)
	if err != nil {
		return nil, err
	}
	return map[string]any{"symbols": syms, "count": len(syms)}, nil
}

// ─── assemble_context ─────────────────────────────────────────────────────────

func (h *Handlers) AssembleContext(params json.RawMessage) (any, error) {
	var p struct {
		Query       string   `json:"query"`
		MaxTokens   int      `json:"max_tokens"`
		ChunkTypes  []string `json:"chunk_types"`
		Language    string   `json:"language"`
		RootPath    string   `json:"root_path"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, err
	}
	if p.Query == "" {
		return nil, fmt.Errorf("query required")
	}
	if p.MaxTokens <= 0 {
		p.MaxTokens = 4000
	}

	root := resolveRoot(coalesce(p.RootPath, h.defaultRoot))
	cfg, _ := config.Load(root)
	store, err := storage.Open(filepath.Join(root, cfg.Storage.Path))
	if err != nil {
		return nil, err
	}
	defer store.Close()

	project, err := store.GetProject(root)
	if err != nil {
		return nil, fmt.Errorf("project not indexed")
	}

	chunkTypes := make([]types.ChunkType, len(p.ChunkTypes))
	for i, ct := range p.ChunkTypes {
		chunkTypes[i] = types.ChunkType(ct)
	}

	results, err := store.Search(storage.SearchRequest{
		ProjectID:  project.ID,
		Query:      p.Query,
		Language:   p.Language,
		ChunkTypes: chunkTypes,
		Limit:      20,
	})
	if err != nil {
		return nil, err
	}

	// Token-budget assembly: ~4 chars per token estimate.
	charsPerToken := 4
	budget := p.MaxTokens * charsPerToken

	var blocks []types.ContextBlock
	usedChunkIDs := map[string]bool{}
	remaining := budget

	for _, r := range results {
		estimate := len(r.Chunk.Content) / charsPerToken
		if remaining <= 0 {
			break
		}
		content := r.Chunk.Content
		if len(content) > remaining*charsPerToken {
			// Truncate to budget.
			content = content[:remaining*charsPerToken]
		}
		chunk := r.Chunk
		chunk.Content = content
		blocks = append(blocks, types.ContextBlock{
			Chunk:         chunk,
			TokenEstimate: estimate,
			Source:        "search",
		})
		usedChunkIDs[r.Chunk.ID] = true
		remaining -= estimate
	}

	totalTokens := 0
	for _, b := range blocks {
		totalTokens += b.TokenEstimate
	}

	return map[string]any{
		"blocks":       blocks,
		"count":        len(blocks),
		"total_tokens": totalTokens,
		"query":        p.Query,
	}, nil
}

// ─── get_dependency_graph ─────────────────────────────────────────────────────

func (h *Handlers) GetDependencyGraph(params json.RawMessage) (any, error) {
	var p struct {
		ChunkID  string `json:"chunk_id"`
		Depth    int    `json:"depth"`
		RootPath string `json:"root_path"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, err
	}
	if p.ChunkID == "" {
		return nil, fmt.Errorf("chunk_id required")
	}
	if p.Depth <= 0 {
		p.Depth = 2
	}

	root := resolveRoot(coalesce(p.RootPath, h.defaultRoot))
	cfg, _ := config.Load(root)
	store, err := storage.Open(filepath.Join(root, cfg.Storage.Path))
	if err != nil {
		return nil, err
	}
	defer store.Close()

	nodes, edges, err := store.GetDependencyGraph(p.ChunkID, p.Depth)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"nodes": nodes,
		"edges": edges,
		"node_count": len(nodes),
		"edge_count": len(edges),
	}, nil
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

// BuildRegistry constructs the default parser registry used by all indexing operations.
// Exported so comparison tests can build an identical registry without duplication.
func BuildRegistry() *parser.Registry {
	reg := parser.NewRegistry()
	reg.Register(terraform.New())
	reg.Register(python.New())
	reg.Register(golang.New())
	reg.Register(typescript.New())
	reg.SetFallback(generic.New())
	return reg
}

func coalesce(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// resolveRoot returns an absolute, OS-normalized path for the given root.
// This ensures MCP callers using forward slashes on Windows still match
// the stored root_path (which was normalized when first indexed).
func resolveRoot(raw string) string {
	abs, err := filepath.Abs(raw)
	if err != nil {
		return raw
	}
	return abs
}
