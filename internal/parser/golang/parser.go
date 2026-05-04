// Package golang provides a regex-based parser for Go source files.
// It extracts functions, methods, structs, and interfaces without CGO or tree-sitter.
package golang

import (
	"crypto/sha256"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/code-index-for-llms/code-index/pkg/types"
)

var (
	// func Name( or func (recv) Name(
	reFunc          = regexp.MustCompile(`^func\s+(?:\([^)]+\)\s+)?([A-Za-z_][A-Za-z0-9_]*)\s*[(\[]`)
	reReceiver      = regexp.MustCompile(`^func\s+\(([^)]+)\)\s+([A-Za-z_][A-Za-z0-9_]*)\s*[(\[]`)
	reTypeStruct    = regexp.MustCompile(`^type\s+([A-Za-z_][A-Za-z0-9_]*)\s+struct\s*\{`)
	reTypeInterface = regexp.MustCompile(`^type\s+([A-Za-z_][A-Za-z0-9_]*)\s+interface\s*\{`)
	reTypeAlias     = regexp.MustCompile(`^type\s+([A-Za-z_][A-Za-z0-9_]*)\s+`)
)

// Parser extracts Go functions, methods, structs, and interfaces.
type Parser struct{}

func New() *Parser { return &Parser{} }

func (p *Parser) Language() string                 { return "go" }
func (p *Parser) Extensions() []string             { return []string{".go"} }
func (p *Parser) CanParse(_ string, _ []byte) bool { return true }

func (p *Parser) SupportedChunkTypes() []types.ChunkType {
	return []types.ChunkType{
		types.ChunkTypeFunction,
		types.ChunkTypeMethod,
		types.ChunkTypeClass, // struct
		types.ChunkTypeInterface,
	}
}

func (p *Parser) Parse(filePath string, content []byte) ([]types.Chunk, error) {
	lines := strings.Split(string(content), "\n")
	now := time.Now()

	type block struct {
		name      string
		kind      types.ChunkType
		startLine int // 1-based
		receiver  string
	}

	var blocks []block

	for i, line := range lines {
		lineNum := i + 1
		trimmed := strings.TrimSpace(line)

		// Skip lines that don't start with func or type at column 0.
		if !strings.HasPrefix(trimmed, "func ") && !strings.HasPrefix(trimmed, "type ") {
			continue
		}

		if m := reReceiver.FindStringSubmatch(trimmed); m != nil {
			// Method with receiver: extract receiver type name.
			recv := extractReceiverType(m[1])
			methodName := m[2]
			blocks = append(blocks, block{
				name:      recv + "." + methodName,
				kind:      types.ChunkTypeMethod,
				startLine: lineNum,
				receiver:  recv,
			})
			continue
		}

		if m := reFunc.FindStringSubmatch(trimmed); m != nil {
			blocks = append(blocks, block{
				name:      m[1],
				kind:      types.ChunkTypeFunction,
				startLine: lineNum,
			})
			continue
		}

		if m := reTypeStruct.FindStringSubmatch(trimmed); m != nil {
			blocks = append(blocks, block{
				name:      m[1],
				kind:      types.ChunkTypeClass,
				startLine: lineNum,
			})
			continue
		}

		if m := reTypeInterface.FindStringSubmatch(trimmed); m != nil {
			blocks = append(blocks, block{
				name:      m[1],
				kind:      types.ChunkTypeInterface,
				startLine: lineNum,
			})
			continue
		}
	}

	// Determine end lines by scanning for matching closing brace.
	chunks := make([]types.Chunk, 0, len(blocks))
	for _, b := range blocks {
		endLine := findGoBlockEnd(lines, b.startLine-1)
		// Trim trailing blank lines within range.
		for endLine > b.startLine && strings.TrimSpace(lines[endLine-1]) == "" {
			endLine--
		}

		body := strings.Join(lines[b.startLine-1:endLine], "\n")
		chunks = append(chunks, makeChunk(filePath, b.name, body, b.startLine, endLine, b.kind, now))
	}

	if len(chunks) == 0 {
		full := strings.Join(lines, "\n")
		chunks = append(chunks, makeChunk(filePath, filePath, full, 1, len(lines), types.ChunkTypeFile, now))
	}

	return chunks, nil
}

func (p *Parser) ExtractSymbols(chunks []types.Chunk) ([]types.Symbol, error) {
	var syms []types.Symbol
	for _, c := range chunks {
		if c.ChunkType == types.ChunkTypeFunction ||
			c.ChunkType == types.ChunkTypeMethod ||
			c.ChunkType == types.ChunkTypeClass ||
			c.ChunkType == types.ChunkTypeInterface {
			syms = append(syms, types.Symbol{
				ID:       fmt.Sprintf("%x", sha256.Sum256([]byte(c.ID+"def"))),
				ChunkID:  c.ID,
				FileID:   c.FileID,
				Name:     c.Name,
				Kind:     types.SymbolKindDefinition,
				Line:     c.StartLine,
				Language: "go",
			})
		}
	}
	return syms, nil
}

// findGoBlockEnd returns the 1-based line number of the closing brace for the
// block whose opening line is at startIdx (0-based). Handles nested braces.
// Falls back to startIdx+1 if no brace found (e.g. single-line type alias).
func findGoBlockEnd(lines []string, startIdx int) int {
	depth := 0
	opened := false
	for i := startIdx; i < len(lines); i++ {
		for _, ch := range lines[i] {
			if ch == '{' {
				depth++
				opened = true
			} else if ch == '}' {
				depth--
				if opened && depth == 0 {
					return i + 1 // 1-based
				}
			}
		}
	}
	// No brace found (e.g. `type Foo = Bar`) - single line.
	return startIdx + 1
}

// extractReceiverType strips pointer and type params: "*MyStruct" → "MyStruct".
func extractReceiverType(recv string) string {
	recv = strings.TrimSpace(recv)
	// Remove variable name prefix: "r *MyStruct" → "*MyStruct"
	parts := strings.Fields(recv)
	typePart := parts[len(parts)-1]
	typePart = strings.TrimPrefix(typePart, "*")
	// Remove generics: "MyStruct[T]" → "MyStruct"
	if idx := strings.Index(typePart, "["); idx != -1 {
		typePart = typePart[:idx]
	}
	return typePart
}

func makeChunk(filePath, name, content string, startLine, endLine int, kind types.ChunkType, now time.Time) types.Chunk {
	id := fmt.Sprintf("%x", sha256.Sum256([]byte(filePath+name+fmt.Sprint(startLine))))
	h := sha256.Sum256([]byte(content))
	return types.Chunk{
		ID:          id,
		FilePath:    filePath,
		Language:    "go",
		ChunkType:   kind,
		Name:        name,
		Content:     content,
		StartLine:   startLine,
		EndLine:     endLine,
		ContentHash: fmt.Sprintf("%x", h),
		IndexedAt:   now,
	}
}
