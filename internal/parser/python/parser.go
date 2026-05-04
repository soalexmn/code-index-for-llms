// Package python provides a regex-based parser for Python source files.
// It extracts functions, classes, and methods without requiring CGO or tree-sitter.
package python

import (
	"crypto/sha256"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/code-index-for-llms/code-index/pkg/types"
)

var (
	reFuncDef  = regexp.MustCompile(`^(\s*)(async\s+)?def\s+([A-Za-z_][A-Za-z0-9_]*)\s*\(`)
	reClassDef = regexp.MustCompile(`^(\s*)class\s+([A-Za-z_][A-Za-z0-9_]*)\s*[:(]`)
	reDecorator = regexp.MustCompile(`^\s*@`)
)

// Parser extracts Python functions, classes, and methods.
type Parser struct{}

func New() *Parser { return &Parser{} }

func (p *Parser) Language() string   { return "python" }
func (p *Parser) Extensions() []string { return []string{".py", ".pyi"} }
func (p *Parser) CanParse(filePath string, content []byte) bool { return true }

func (p *Parser) SupportedChunkTypes() []types.ChunkType {
	return []types.ChunkType{
		types.ChunkTypeFunction,
		types.ChunkTypeClass,
		types.ChunkTypeMethod,
	}
}

func (p *Parser) Parse(filePath string, content []byte) ([]types.Chunk, error) {
	lines := strings.Split(string(content), "\n")
	now := time.Now()

	// Collect top-level and class-level blocks.
	type block struct {
		name      string
		kind      types.ChunkType
		startLine int // 1-based
		indent    int // indentation level of the def/class keyword line
		classCtx  string // non-empty if inside a class (for METHOD detection)
	}

	var blocks []block

	// Track class nesting so we can mark methods.
	// classStack holds (className, indentLevel) for open class bodies.
	type classEntry struct {
		name   string
		indent int
	}
	var classStack []classEntry

	// Collect decorator start lines so we can extend the chunk start upward.
	decoratorStart := -1

	for i, line := range lines {
		lineNum := i + 1

		// Pop classes whose indent level >= current line's indent (class ended).
		indent := leadingSpaces(line)
		if strings.TrimSpace(line) != "" {
			for len(classStack) > 0 && indent <= classStack[len(classStack)-1].indent {
				classStack = classStack[:len(classStack)-1]
			}
		}

		if reDecorator.MatchString(line) {
			if decoratorStart == -1 {
				decoratorStart = lineNum
			}
			continue
		}

		if m := reClassDef.FindStringSubmatch(line); m != nil {
			start := lineNum
			if decoratorStart != -1 {
				start = decoratorStart
			}
			className := m[2]
			classIndent := leadingSpaces(line)
			classStack = append(classStack, classEntry{name: className, indent: classIndent})
			blocks = append(blocks, block{
				name:      className,
				kind:      types.ChunkTypeClass,
				startLine: start,
				indent:    classIndent,
			})
			decoratorStart = -1
			continue
		}

		if m := reFuncDef.FindStringSubmatch(line); m != nil {
			start := lineNum
			if decoratorStart != -1 {
				start = decoratorStart
			}
			funcName := m[3]
			funcIndent := leadingSpaces(line)

			kind := types.ChunkTypeFunction
			classCtx := ""
			if len(classStack) > 0 {
				kind = types.ChunkTypeMethod
				classCtx = classStack[len(classStack)-1].name
			}

			blocks = append(blocks, block{
				name:      funcName,
				kind:      kind,
				startLine: start,
				indent:    funcIndent,
				classCtx:  classCtx,
			})
			decoratorStart = -1
			continue
		}

		// Non-empty, non-decorator line resets decorator accumulator.
		if strings.TrimSpace(line) != "" {
			decoratorStart = -1
		}
	}

	// Determine end lines: each block ends just before the next block
	// at the same or lesser indentation level, or at EOF.
	chunks := make([]types.Chunk, 0, len(blocks))
	for i, b := range blocks {
		endLine := len(lines)
		for j := i + 1; j < len(blocks); j++ {
			if blocks[j].indent <= b.indent {
				endLine = blocks[j].startLine - 1
				break
			}
		}
		// Trim trailing blank lines.
		for endLine > b.startLine && strings.TrimSpace(lines[endLine-1]) == "" {
			endLine--
		}

		content := strings.Join(lines[b.startLine-1:endLine], "\n")

		qualName := b.name
		if b.classCtx != "" {
			qualName = b.classCtx + "." + b.name
		}

		chunks = append(chunks, makeChunk(filePath, qualName, content, b.startLine, endLine, b.kind, now))
	}

	// Fallback: emit whole file if no blocks found.
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
			c.ChunkType == types.ChunkTypeClass ||
			c.ChunkType == types.ChunkTypeMethod {
			syms = append(syms, types.Symbol{
				ID:       fmt.Sprintf("%x", sha256.Sum256([]byte(c.ID+"def"))),
				ChunkID:  c.ID,
				FileID:   c.FileID,
				Name:     c.Name,
				Kind:     types.SymbolKindDefinition,
				Line:     c.StartLine,
				Language: "python",
			})
		}
	}
	return syms, nil
}

func leadingSpaces(s string) int {
	count := 0
	for _, r := range s {
		if r == ' ' {
			count++
		} else if r == '\t' {
			count += 4
		} else {
			break
		}
	}
	return count
}

func makeChunk(filePath, name, content string, startLine, endLine int, kind types.ChunkType, now time.Time) types.Chunk {
	id := fmt.Sprintf("%x", sha256.Sum256([]byte(filePath+name+fmt.Sprint(startLine))))
	h := sha256.Sum256([]byte(content))
	return types.Chunk{
		ID:          id,
		FilePath:    filePath,
		Language:    "python",
		ChunkType:   kind,
		Name:        name,
		Content:     content,
		StartLine:   startLine,
		EndLine:     endLine,
		ContentHash: fmt.Sprintf("%x", h),
		IndexedAt:   now,
	}
}
