// Package generic provides a line-based fallback parser for file types
// that have no dedicated AST parser. It splits files into fixed-size
// overlapping line windows and emits a single FILE chunk per file.
package generic

import (
	"crypto/sha256"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/code-index-for-llms/code-index/pkg/types"
)

const (
	defaultWindowLines  = 60
	defaultOverlapLines = 10
)

// Parser is the line-based fallback. It claims no extensions - it is
// always registered as the registry fallback, not as a named language parser.
type Parser struct {
	WindowLines  int
	OverlapLines int
}

func New() *Parser {
	return &Parser{
		WindowLines:  defaultWindowLines,
		OverlapLines: defaultOverlapLines,
	}
}

func (p *Parser) Language() string                 { return "generic" }
func (p *Parser) Extensions() []string             { return nil }
func (p *Parser) CanParse(_ string, _ []byte) bool { return true }

func (p *Parser) SupportedChunkTypes() []types.ChunkType {
	return []types.ChunkType{types.ChunkTypeFile}
}

func (p *Parser) Parse(filePath string, content []byte) ([]types.Chunk, error) {
	lines := strings.Split(string(content), "\n")
	name := filepath.Base(filePath)
	now := time.Now()

	// Small file: emit as single chunk.
	if len(lines) <= p.WindowLines {
		return []types.Chunk{makeChunk(filePath, name, content, 1, len(lines), now)}, nil
	}

	// Large file: sliding window with overlap.
	var chunks []types.Chunk
	step := p.WindowLines - p.OverlapLines
	for start := 0; start < len(lines); start += step {
		end := start + p.WindowLines
		if end > len(lines) {
			end = len(lines)
		}
		window := strings.Join(lines[start:end], "\n")
		chunkName := fmt.Sprintf("%s:%d-%d", name, start+1, end)
		chunks = append(chunks, makeChunk(filePath, chunkName, []byte(window), start+1, end, now))
		if end == len(lines) {
			break
		}
	}
	return chunks, nil
}

func (p *Parser) ExtractSymbols(_ []types.Chunk) ([]types.Symbol, error) {
	return nil, nil // Generic parser has no symbol intelligence.
}

func makeChunk(filePath, name string, content []byte, startLine, endLine int, now time.Time) types.Chunk {
	h := sha256.Sum256(content)
	return types.Chunk{
		ID:          fmt.Sprintf("%x", sha256.Sum256([]byte(filePath+fmt.Sprint(startLine)))),
		FilePath:    filePath,
		Language:    "generic",
		ChunkType:   types.ChunkTypeFile,
		Name:        name,
		Content:     string(content),
		StartLine:   startLine,
		EndLine:     endLine,
		ContentHash: fmt.Sprintf("%x", h),
		IndexedAt:   now,
	}
}
