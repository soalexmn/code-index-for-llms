// Package typescript provides a regex-based parser for TypeScript and JavaScript.
// Extracts functions, classes, methods, and interfaces without CGO or tree-sitter.
package typescript

import (
	"crypto/sha256"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/code-index-for-llms/code-index/pkg/types"
)

var (
	// class Foo / export class Foo / export default class Foo / abstract class Foo
	reClass = regexp.MustCompile(`^(?:export\s+)?(?:default\s+)?(?:abstract\s+)?class\s+([A-Za-z_$][A-Za-z0-9_$]*)\s*(?:<[^{]*>)?\s*(?:extends|implements|{)`)

	// interface Foo / export interface Foo
	reInterface = regexp.MustCompile(`^(?:export\s+)?interface\s+([A-Za-z_$][A-Za-z0-9_$]*)\s*(?:<[^{]*>)?\s*(?:extends\s+[^{]+)?\{`)

	// function foo( / export function foo( / export default function foo( / async function foo(
	reFunction = regexp.MustCompile(`^(?:export\s+)?(?:default\s+)?(?:async\s+)?function\s*\*?\s+([A-Za-z_$][A-Za-z0-9_$]*)\s*[(<]`)

	// Arrow function: const foo = (...) => / export const foo = async (...) =>
	reArrowFunc = regexp.MustCompile(`^(?:export\s+)?(?:const|let|var)\s+([A-Za-z_$][A-Za-z0-9_$]*)\s*(?::[^=]+)?\s*=\s*(?:async\s+)?\(`)

	// Method inside class: foo( / async foo( / private foo( / static foo( / get foo( / set foo(
	reMethod = regexp.MustCompile(`^\s+(?:(?:public|private|protected|static|async|readonly|abstract|override)\s+)*(?:get\s+|set\s+)?([A-Za-z_$][A-Za-z0-9_$]*)\s*[(<]`)

	// Decorator
	reDecorator = regexp.MustCompile(`^\s*@[A-Za-z_$]`)
)

// Parser handles TypeScript (.ts, .tsx) and JavaScript (.js, .jsx, .mjs).
type Parser struct{}

func New() *Parser { return &Parser{} }

func (p *Parser) Language() string { return "typescript" }
func (p *Parser) Extensions() []string {
	return []string{".ts", ".tsx", ".js", ".jsx", ".mjs", ".cjs"}
}
func (p *Parser) CanParse(_ string, _ []byte) bool { return true }

func (p *Parser) SupportedChunkTypes() []types.ChunkType {
	return []types.ChunkType{
		types.ChunkTypeFunction,
		types.ChunkTypeClass,
		types.ChunkTypeMethod,
		types.ChunkTypeInterface,
	}
}

func (p *Parser) Parse(filePath string, content []byte) ([]types.Chunk, error) {
	lines := strings.Split(string(content), "\n")
	now := time.Now()
	lang := p.langForFile(filePath)

	type block struct {
		name      string
		kind      types.ChunkType
		startLine int // 1-based
		indent    int
		classCtx  string
	}

	var blocks []block

	type classEntry struct {
		name       string
		startLine  int
		braceDepth int
	}
	var classStack []classEntry
	braceDepth := 0
	decoratorStart := -1
	decoratorParens := 0 // tracks open parens inside multi-line decorators

	for i, line := range lines {
		lineNum := i + 1
		trimmed := strings.TrimSpace(line)
		indent := leadingSpaces(line)

		// Track brace depth to know when we leave a class body.
		for _, ch := range line {
			if ch == '{' {
				braceDepth++
			} else if ch == '}' {
				braceDepth--
				// Pop class if we've closed back to its opening depth.
				if len(classStack) > 0 && braceDepth < classStack[len(classStack)-1].braceDepth {
					classStack = classStack[:len(classStack)-1]
				}
			}
		}

		if reDecorator.MatchString(line) {
			if decoratorStart == -1 {
				decoratorStart = lineNum
				decoratorParens = 0
			}
			for _, ch := range line {
				if ch == '(' {
					decoratorParens++
				} else if ch == ')' {
					decoratorParens--
				}
			}
			continue
		}

		// Still inside multi-line decorator arguments (e.g. @Component({ ... })).
		if decoratorStart != -1 && decoratorParens > 0 {
			for _, ch := range line {
				if ch == '(' {
					decoratorParens++
				} else if ch == ')' {
					decoratorParens--
				}
			}
			continue
		}

		if m := reClass.FindStringSubmatch(trimmed); m != nil {
			start := lineNum
			if decoratorStart != -1 {
				start = decoratorStart
			}
			className := m[1]
			classStack = append(classStack, classEntry{name: className, startLine: lineNum, braceDepth: braceDepth})
			blocks = append(blocks, block{name: className, kind: types.ChunkTypeClass, startLine: start, indent: indent})
			decoratorStart = -1
			continue
		}

		if m := reInterface.FindStringSubmatch(trimmed); m != nil {
			blocks = append(blocks, block{name: m[1], kind: types.ChunkTypeInterface, startLine: lineNum, indent: indent})
			decoratorStart = -1
			continue
		}

		if m := reFunction.FindStringSubmatch(trimmed); m != nil {
			start := lineNum
			if decoratorStart != -1 {
				start = decoratorStart
			}
			blocks = append(blocks, block{name: m[1], kind: types.ChunkTypeFunction, startLine: start, indent: indent})
			decoratorStart = -1
			continue
		}

		if m := reArrowFunc.FindStringSubmatch(trimmed); m != nil && len(classStack) == 0 {
			blocks = append(blocks, block{name: m[1], kind: types.ChunkTypeFunction, startLine: lineNum, indent: indent})
			decoratorStart = -1
			continue
		}

		// Method detection: indented line inside a class body.
		if len(classStack) > 0 && indent > 0 {
			if m := reMethod.FindStringSubmatch(line); m != nil {
				name := m[1]
				// Skip keywords that look like methods.
				if !isKeyword(name) {
					start := lineNum
					if decoratorStart != -1 {
						start = decoratorStart
					}
					classCtx := classStack[len(classStack)-1].name
					blocks = append(blocks, block{
						name:      classCtx + "." + name,
						kind:      types.ChunkTypeMethod,
						startLine: start,
						indent:    indent,
						classCtx:  classCtx,
					})
				}
			}
		}

		if trimmed != "" {
			decoratorStart = -1
		}
	}

	// Determine end lines by brace tracking from the start line.
	chunks := make([]types.Chunk, 0, len(blocks))
	for i, b := range blocks {
		endLine := len(lines)
		// Next block at same or lesser indent ends this one.
		for j := i + 1; j < len(blocks); j++ {
			if blocks[j].indent <= b.indent && blocks[j].classCtx == b.classCtx {
				endLine = blocks[j].startLine - 1
				break
			}
		}
		for endLine > b.startLine && strings.TrimSpace(lines[endLine-1]) == "" {
			endLine--
		}
		body := strings.Join(lines[b.startLine-1:endLine], "\n")
		chunks = append(chunks, makeChunk(filePath, b.name, body, b.startLine, endLine, b.kind, lang, now))
	}

	if len(chunks) == 0 {
		full := strings.Join(lines, "\n")
		chunks = append(chunks, makeChunk(filePath, filePath, full, 1, len(lines), types.ChunkTypeFile, lang, now))
	}
	return chunks, nil
}

func (p *Parser) ExtractSymbols(chunks []types.Chunk) ([]types.Symbol, error) {
	lang := "typescript"
	var syms []types.Symbol
	for _, c := range chunks {
		if c.ChunkType == types.ChunkTypeFunction ||
			c.ChunkType == types.ChunkTypeClass ||
			c.ChunkType == types.ChunkTypeMethod ||
			c.ChunkType == types.ChunkTypeInterface {
			syms = append(syms, types.Symbol{
				ID:       fmt.Sprintf("%x", sha256.Sum256([]byte(c.ID+"def"))),
				ChunkID:  c.ID,
				FileID:   c.FileID,
				Name:     c.Name,
				Kind:     types.SymbolKindDefinition,
				Line:     c.StartLine,
				Language: lang,
			})
		}
	}
	return syms, nil
}

// langForFile returns "javascript" for .js/.jsx/.mjs/.cjs, "typescript" otherwise.
func (p *Parser) langForFile(filePath string) string {
	lower := strings.ToLower(filePath)
	for _, ext := range []string{".js", ".jsx", ".mjs", ".cjs"} {
		if strings.HasSuffix(lower, ext) {
			return "javascript"
		}
	}
	return "typescript"
}

func leadingSpaces(s string) int {
	count := 0
	for _, r := range s {
		if r == ' ' {
			count++
		} else if r == '\t' {
			count += 2
		} else {
			break
		}
	}
	return count
}

func isKeyword(name string) bool {
	switch name {
	case "if", "else", "for", "while", "switch", "try", "catch", "finally",
		"return", "throw", "new", "delete", "typeof", "instanceof",
		"constructor", "super", "this":
		return true
	}
	return false
}

func makeChunk(filePath, name, content string, startLine, endLine int, kind types.ChunkType, lang string, now time.Time) types.Chunk {
	id := fmt.Sprintf("%x", sha256.Sum256([]byte(filePath+name+fmt.Sprint(startLine))))
	h := sha256.Sum256([]byte(content))
	return types.Chunk{
		ID:          id,
		FilePath:    filePath,
		Language:    lang,
		ChunkType:   kind,
		Name:        name,
		Content:     content,
		StartLine:   startLine,
		EndLine:     endLine,
		ContentHash: fmt.Sprintf("%x", h),
		IndexedAt:   now,
	}
}
