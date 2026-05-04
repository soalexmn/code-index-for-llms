package storage

import "github.com/code-index-for-llms/code-index/pkg/types"

// Store is the persistence interface. The SQLite implementation is the only
// built-in implementation; the interface exists to keep handlers testable.
type Store interface {
	// Project operations
	UpsertProject(project types.Project) error
	GetProject(rootPath string) (types.Project, error)

	// File operations
	UpsertFile(file types.File) error
	GetFile(projectID, relativePath string) (types.File, bool, error)
	ListFiles(projectID string) ([]types.File, error)
	DeleteFile(fileID string) error

	// Chunk operations
	UpsertChunks(chunks []types.Chunk) error
	GetChunk(id string) (types.Chunk, error)
	DeleteChunksByFile(fileID string) error

	// Symbol operations
	UpsertSymbols(symbols []types.Symbol) error
	DeleteSymbolsByFile(fileID string) error
	GetFileSymbols(fileID string) ([]types.Symbol, error)
	FindReferences(projectID, name string) ([]types.Symbol, error)

	// Search
	Search(req SearchRequest) ([]types.SearchResult, error)

	// IaC-specific query
	QueryResources(req ResourceQuery) ([]types.Chunk, error)

	// Graph
	UpsertGraphEdges(edges []types.GraphEdge) error
	GetDependencyGraph(chunkID string, depth int) ([]types.GraphNode, []types.GraphEdge, error)

	// Stats
	IndexStatus(projectID string) (types.IndexStatus, error)

	// Lifecycle
	Migrate() error
	Close() error
}

// SearchRequest carries parameters for a hybrid vector+BM25 search.
type SearchRequest struct {
	ProjectID   string
	Query       string
	QueryVector []float32 // Nil = BM25 only
	Language    string    // Empty = all languages
	ChunkTypes  []types.ChunkType
	Limit       int
	// HybridWeight controls vector vs BM25 blend: 0.0 = pure BM25, 1.0 = pure vector.
	// Only meaningful when QueryVector is non-nil.
	HybridWeight float64
}

// ResourceQuery filters IaC resource chunks.
type ResourceQuery struct {
	ProjectID    string
	ResourceType string // e.g. "aws_s3_bucket"
	Provider     string // e.g. "aws"
	ModulePath   string // e.g. "module.networking"
	Limit        int
}
