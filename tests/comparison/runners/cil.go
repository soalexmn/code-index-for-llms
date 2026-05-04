// Package runners provides Runner implementations for the comparison benchmark.
package runners

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/code-index-for-llms/code-index/internal/config"
	"github.com/code-index-for-llms/code-index/internal/indexer"
	"github.com/code-index-for-llms/code-index/internal/mcp/handlers"
	"github.com/code-index-for-llms/code-index/internal/storage"
	"github.com/code-index-for-llms/code-index/pkg/types"
)

// CILRunner (code-index-for-llms) indexes the fixture via the Go API and
// searches using SQLiteStore.Search() directly - no subprocess overhead.
type CILRunner struct {
	dbPath    string
	projectID string
	store     storage.Store
	counter   TokenCounter
}

// NewCILRunner creates a CILRunner with the given token counter.
// dbPath is the path to write the test SQLite database.
func NewCILRunner(dbPath string, counter TokenCounter) *CILRunner {
	return &CILRunner{dbPath: dbPath, counter: counter}
}

func (r *CILRunner) Name() string { return "cil" }

// Setup builds a fresh index of the fixture directory using the same
// parser registry and indexing logic as the production MCP server.
func (r *CILRunner) Setup(fixtureDir string) error {
	// Remove any stale index from a previous run.
	_ = os.Remove(r.dbPath)

	store, err := storage.Open(r.dbPath)
	if err != nil {
		return fmt.Errorf("cil open store: %w", err)
	}
	if err := store.Migrate(); err != nil {
		store.Close()
		return fmt.Errorf("cil migrate: %w", err)
	}
	r.store = store

	// Build the project record.
	projectID := indexer.ProjectID(fixtureDir)
	r.projectID = projectID

	project := types.Project{
		ID:             projectID,
		RootPath:       fixtureDir,
		Name:           filepath.Base(fixtureDir),
		EmbeddingModel: "none",
	}
	if err := store.UpsertProject(project); err != nil {
		return fmt.Errorf("cil upsert project: %w", err)
	}

	// Load config from fixture directory (uses .code-index.yaml if present).
	cfg, _ := config.Load(fixtureDir)

	// Walk + chunk using the same registry as production.
	reg := handlers.BuildRegistry()
	chunker := indexer.NewChunker(reg)

	files, err := indexer.Walk(fixtureDir, cfg)
	if err != nil {
		return fmt.Errorf("cil walk: %w", err)
	}

	langSet := map[string]bool{}
	totalChunks := 0

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
		}

		chunks, symbols, err := chunker.ChunkFile(fileID, projectID, fe)
		if err != nil {
			continue
		}
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
			_ = store.UpsertSymbols(symbols)
		}
	}

	// Update project with final counts.
	langs := make([]string, 0, len(langSet))
	for l := range langSet {
		langs = append(langs, l)
	}
	project.Languages = langs
	project.TotalFiles = len(files)
	project.TotalChunks = totalChunks
	_ = store.UpsertProject(project)

	return nil
}

// Search calls the SQLite BM25 search directly (no subprocess).
func (r *CILRunner) Search(query string, topK int) ([]types.SearchResult, time.Duration, error) {
	start := time.Now()
	results, err := r.store.Search(storage.SearchRequest{
		ProjectID: r.projectID,
		Query:     query,
		Limit:     topK,
	})
	return results, time.Since(start), err
}

// ContextTokens sums the token count of all retrieved chunk contents.
func (r *CILRunner) ContextTokens(results []types.SearchResult) int {
	total := 0
	for _, res := range results {
		total += r.counter.Count(res.Chunk.Content)
	}
	return total
}

// OutputTokensEstimate returns the baseline unchanged (CIL does not compress output).
func (r *CILRunner) OutputTokensEstimate(baselineOutputTokens int) int {
	return baselineOutputTokens
}

// Cleanup closes the SQLite store and removes the temporary DB file.
func (r *CILRunner) Cleanup() error {
	if r.store != nil {
		if err := r.store.Close(); err != nil {
			return err
		}
	}
	_ = os.Remove(r.dbPath)
	return nil
}
