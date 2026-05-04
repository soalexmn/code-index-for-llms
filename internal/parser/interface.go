package parser

import "github.com/code-index-for-llms/code-index/pkg/types"

// LanguageParser extracts semantic chunks and symbols from source files.
// Implement this interface to add support for a new language.
// Register the implementation in registry.go with one line: registry.Register(&MyParser{}).
type LanguageParser interface {
	// Language returns the canonical identifier, e.g. "terraform", "python", "go".
	Language() string

	// Extensions returns file extensions this parser claims, e.g. [".tf", ".tfvars"].
	Extensions() []string

	// CanParse performs content-based detection when extension matching is ambiguous
	// (shebang lines, magic bytes, etc.). Return false to fall through to the next parser.
	CanParse(filePath string, content []byte) bool

	// Parse extracts chunks from source content.
	// filePath is relative to the project root.
	// Returns chunks in document order (top-to-bottom).
	Parse(filePath string, content []byte) ([]types.Chunk, error)

	// ExtractSymbols returns the symbol table for cross-reference indexing.
	// Called after Parse; receives the chunks produced by Parse.
	ExtractSymbols(chunks []types.Chunk) ([]types.Symbol, error)

	// SupportedChunkTypes declares which ChunkTypes this parser emits.
	// Used to guide UI and query filtering.
	SupportedChunkTypes() []types.ChunkType
}
