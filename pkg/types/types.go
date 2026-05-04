package types

import "time"

// ChunkType classifies the semantic role of a code chunk.
type ChunkType string

const (
	ChunkTypeFunction  ChunkType = "FUNCTION"
	ChunkTypeClass     ChunkType = "CLASS"
	ChunkTypeMethod    ChunkType = "METHOD"
	ChunkTypeInterface ChunkType = "INTERFACE"
	ChunkTypeVariable  ChunkType = "VARIABLE"
	ChunkTypeResource  ChunkType = "RESOURCE" // Terraform resource block
	ChunkTypeModule    ChunkType = "MODULE"   // Terraform module block
	ChunkTypeBlock     ChunkType = "BLOCK"    // HCL provider/terraform/locals block
	ChunkTypeImport    ChunkType = "IMPORT"
	ChunkTypeComment   ChunkType = "COMMENT"
	ChunkTypeFile      ChunkType = "FILE" // Whole-file fallback
)

// SymbolKind classifies how a symbol appears in source.
type SymbolKind string

const (
	SymbolKindDefinition SymbolKind = "DEFINITION"
	SymbolKindReference  SymbolKind = "REFERENCE"
	SymbolKindImport     SymbolKind = "IMPORT"
)

// Chunk is the fundamental unit of indexed content.
type Chunk struct {
	ID            string
	FileID        string
	ProjectID     string
	FilePath      string            // Relative to project root
	Language      string
	ChunkType     ChunkType
	Name          string            // Symbol name: "aws_s3_bucket.main", "MyClass", "parse_config"
	Content       string
	StartLine     int
	EndLine       int
	ParentChunkID string            // Non-empty for nested structures (method inside class)
	Embedding     []float32         // Nil until embedded
	Metadata      map[string]string // Language-specific extras; TF: resource_type, provider, resource_name
	ContentHash   string            // SHA256 of Content, used for incremental updates
	IndexedAt     time.Time
}

// Symbol is a named entity for cross-reference indexing.
type Symbol struct {
	ID      string
	ChunkID string
	FileID  string
	Name    string
	Kind    SymbolKind
	Line    int
	Language string
}

// File represents an indexed source file.
type File struct {
	ID           string
	ProjectID    string
	RelativePath string
	AbsolutePath string
	Language     string
	SizeBytes    int64
	ContentHash  string
	LastModified time.Time
	IndexedAt    time.Time
	ChunkCount   int
	IsIgnored    bool
}

// Project holds metadata about an indexed codebase.
type Project struct {
	ID             string
	RootPath       string
	Name           string
	CreatedAt      time.Time
	LastIndexedAt  time.Time
	TotalFiles     int
	TotalChunks    int
	Languages      []string
	EmbeddingModel string
	SchemaVersion  int
}

// SearchResult is a ranked chunk returned by hybrid search.
type SearchResult struct {
	Chunk       Chunk
	Score       float64 // RRF combined score
	VectorScore float64 // Cosine similarity component
	BM25Score   float64 // BM25 text match component
}

// GraphNode represents a logical code entity in the dependency graph.
type GraphNode struct {
	ID       string
	ChunkID  string
	Name     string
	Kind     ChunkType
	FilePath string
	Language string
}

// EdgeKind classifies the relationship between two graph nodes.
type EdgeKind string

const (
	EdgeKindCalls   EdgeKind = "calls"
	EdgeKindImports EdgeKind = "imports"
	EdgeKindDefines EdgeKind = "defines"
	EdgeKindExtends EdgeKind = "extends"
	EdgeKindUses    EdgeKind = "uses" // e.g. var.foo, module.bar
)

// GraphEdge is a directed relationship between two GraphNodes.
type GraphEdge struct {
	ID       string
	FromID   string
	ToID     string
	Kind     EdgeKind
	FilePath string // File where the relationship is expressed
	Line     int
}

// ContextBlock is a token-budgeted unit returned by assemble_context.
type ContextBlock struct {
	Chunk          Chunk
	TokenEstimate  int
	Source         string // "search", "related", "dependency"
}

// IndexStatus is the health/stats snapshot returned by get_index_status.
type IndexStatus struct {
	Project       Project
	TotalFiles    int
	TotalChunks   int
	Languages     []string
	LastIndexed   time.Time
	IsStale       bool
	EmbeddingModel string
}
