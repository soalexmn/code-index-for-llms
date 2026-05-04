package indexer

import (
	"crypto/sha256"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/code-index-for-llms/code-index/internal/parser"
	"github.com/code-index-for-llms/code-index/pkg/types"
)

// Chunker orchestrates parsing → chunking for indexed files.
type Chunker struct {
	registry *parser.Registry
}

func NewChunker(registry *parser.Registry) *Chunker {
	return &Chunker{registry: registry}
}

// ChunkFile parses a single FileEntry into Chunks and Symbols.
// fileID and projectID are set on the returned chunks.
func (c *Chunker) ChunkFile(fileID, projectID string, entry FileEntry) ([]types.Chunk, []types.Symbol, error) {
	p := c.registry.Detect(entry.RelPath, entry.Content)

	chunks, err := p.Parse(entry.RelPath, entry.Content)
	if err != nil {
		// Fall back to generic chunker on parse errors.
		fallback := c.registry.GetFallback()
		if fallback != nil {
			chunks, err = fallback.Parse(entry.RelPath, entry.Content)
		}
		if err != nil {
			return nil, nil, err
		}
	}

	lang := p.Language()
	now := time.Now()

	for i := range chunks {
		chunks[i].FileID = fileID
		chunks[i].FilePath = entry.RelPath
		chunks[i].Language = lang
		chunks[i].IndexedAt = now

		// Ensure ID is set if parser left it empty.
		if chunks[i].ID == "" {
			h := sha256.Sum256([]byte(fmt.Sprintf("%s:%d:%s", entry.RelPath, chunks[i].StartLine, chunks[i].Content)))
			chunks[i].ID = fmt.Sprintf("%x", h)[:16]
		}
		// Ensure ContentHash.
		if chunks[i].ContentHash == "" {
			h := sha256.Sum256([]byte(chunks[i].Content))
			chunks[i].ContentHash = fmt.Sprintf("%x", h)
		}
	}

	symbols, err := p.ExtractSymbols(chunks)
	if err != nil {
		symbols = nil // Non-fatal; indexing continues without symbols.
	}
	for i := range symbols {
		symbols[i].FileID = fileID
		if symbols[i].Language == "" {
			symbols[i].Language = lang
		}
	}

	return chunks, symbols, nil
}

// DetectLanguage returns the language name for a file path + content.
func (c *Chunker) DetectLanguage(relPath string, content []byte) string {
	p := c.registry.Detect(relPath, content)
	return p.Language()
}

// fileID generates a stable ID for a file within a project.
func FileID(projectID, relPath string) string {
	h := sha256.Sum256([]byte(projectID + ":" + relPath))
	return fmt.Sprintf("%x", h)[:16]
}

// projectID generates a stable ID for a project root path.
func ProjectID(rootPath string) string {
	cleaned := filepath.ToSlash(strings.TrimRight(rootPath, "/\\"))
	h := sha256.Sum256([]byte(cleaned))
	return fmt.Sprintf("%x", h)[:16]
}
