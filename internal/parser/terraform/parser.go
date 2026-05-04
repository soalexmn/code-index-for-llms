// Package terraform parses Terraform/OpenTofu HCL files into semantic chunks.
// Each block type is mapped to a specific ChunkType with structured Metadata,
// enabling IaC-aware queries (e.g. "find all S3 bucket resources").
package terraform

import (
	"crypto/sha256"
	"fmt"
	"strings"
	"time"

	"github.com/code-index-for-llms/code-index/pkg/types"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/zclconf/go-cty/cty"
)

// Parser handles .tf and .tfvars files.
type Parser struct{}

func New() *Parser { return &Parser{} }

func (p *Parser) Language() string { return "terraform" }

func (p *Parser) Extensions() []string { return []string{".tf", ".tfvars"} }

func (p *Parser) CanParse(_ string, _ []byte) bool { return true }

func (p *Parser) SupportedChunkTypes() []types.ChunkType {
	return []types.ChunkType{
		types.ChunkTypeResource,
		types.ChunkTypeModule,
		types.ChunkTypeVariable,
		types.ChunkTypeBlock,
	}
}

func (p *Parser) Parse(filePath string, content []byte) ([]types.Chunk, error) {
	file, diags := hclsyntax.ParseConfig(content, filePath, hcl.Pos{Line: 1, Column: 1})
	if diags.HasErrors() && file == nil {
		return nil, fmt.Errorf("HCL parse error in %s: %s", filePath, diags.Error())
	}

	body, ok := file.Body.(*hclsyntax.Body)
	if !ok {
		return nil, fmt.Errorf("unexpected HCL body type in %s", filePath)
	}

	now := time.Now()
	var chunks []types.Chunk

	for _, block := range body.Blocks {
		chunk, err := blockToChunk(filePath, block, content, now)
		if err != nil {
			continue // Skip unparseable blocks, don't abort entire file.
		}
		if chunk != nil {
			chunks = append(chunks, *chunk)
		}
	}

	return chunks, nil
}

func (p *Parser) ExtractSymbols(chunks []types.Chunk) ([]types.Symbol, error) {
	var symbols []types.Symbol
	for _, chunk := range chunks {
		// Emit a DEFINITION symbol for every named chunk.
		if chunk.Name != "" {
			symbols = append(symbols, types.Symbol{
				ID:       chunkSymbolID(chunk),
				ChunkID:  chunk.ID,
				FileID:   chunk.FileID,
				Name:     chunk.Name,
				Kind:     types.SymbolKindDefinition,
				Line:     chunk.StartLine,
				Language: "terraform",
			})
		}

		// Extract references from content: var.X, module.X.Y, local.X, data.TYPE.NAME
		refs := extractReferences(chunk.Content)
		for _, ref := range refs {
			symbols = append(symbols, types.Symbol{
				ID:       refSymbolID(chunk, ref),
				ChunkID:  chunk.ID,
				FileID:   chunk.FileID,
				Name:     ref,
				Kind:     types.SymbolKindReference,
				Line:     chunk.StartLine,
				Language: "terraform",
			})
		}
	}
	return symbols, nil
}

// blockToChunk converts one HCL block into a Chunk.
func blockToChunk(filePath string, block *hclsyntax.Block, fileContent []byte, now time.Time) (*types.Chunk, error) {
	startLine := block.DefRange().Start.Line
	endLine := block.CloseBraceRange.End.Line

	// Extract raw source text for this block.
	content := extractRange(fileContent, startLine, endLine)

	meta := map[string]string{}
	var chunkType types.ChunkType
	var name string

	switch block.Type {
	case "resource":
		if len(block.Labels) < 2 {
			return nil, nil
		}
		chunkType = types.ChunkTypeResource
		resourceType := block.Labels[0]
		resourceName := block.Labels[1]
		name = resourceType + "." + resourceName
		meta["resource_type"] = resourceType
		meta["resource_name"] = resourceName
		meta["provider"] = providerFromResourceType(resourceType)

	case "module":
		if len(block.Labels) < 1 {
			return nil, nil
		}
		chunkType = types.ChunkTypeModule
		name = "module." + block.Labels[0]
		meta["module_name"] = block.Labels[0]
		if src := attrStringValue(block, "source"); src != "" {
			meta["source"] = src
		}

	case "variable":
		if len(block.Labels) < 1 {
			return nil, nil
		}
		chunkType = types.ChunkTypeVariable
		name = "var." + block.Labels[0]
		meta["var_name"] = block.Labels[0]
		if t := attrStringValue(block, "type"); t != "" {
			meta["type"] = t
		}
		if d := attrStringValue(block, "default"); d != "" {
			meta["default"] = d
		}

	case "output":
		if len(block.Labels) < 1 {
			return nil, nil
		}
		chunkType = types.ChunkTypeVariable
		name = "output." + block.Labels[0]
		meta["output_name"] = block.Labels[0]

	case "locals":
		// Emit one VARIABLE chunk per attribute in the locals block.
		// Caller handles this specially - we return the whole block as one chunk
		// and let the symbol extractor enumerate attributes.
		chunkType = types.ChunkTypeBlock
		name = "locals"

	case "provider":
		chunkType = types.ChunkTypeBlock
		if len(block.Labels) >= 1 {
			name = "provider." + block.Labels[0]
			meta["provider_name"] = block.Labels[0]
		} else {
			name = "provider"
		}

	case "terraform":
		chunkType = types.ChunkTypeBlock
		name = "terraform"

	case "data":
		if len(block.Labels) < 2 {
			return nil, nil
		}
		chunkType = types.ChunkTypeResource
		name = "data." + block.Labels[0] + "." + block.Labels[1]
		meta["resource_type"] = block.Labels[0]
		meta["resource_name"] = block.Labels[1]
		meta["is_data_source"] = "true"

	default:
		// Unknown block type - emit as generic BLOCK.
		chunkType = types.ChunkTypeBlock
		name = block.Type
		if len(block.Labels) > 0 {
			name += "." + strings.Join(block.Labels, ".")
		}
	}

	id := chunkID(filePath, startLine, content)
	h := sha256.Sum256([]byte(content))

	return &types.Chunk{
		ID:          id,
		FilePath:    filePath,
		Language:    "terraform",
		ChunkType:   chunkType,
		Name:        name,
		Content:     content,
		StartLine:   startLine,
		EndLine:     endLine,
		Metadata:    meta,
		ContentHash: fmt.Sprintf("%x", h),
		IndexedAt:   now,
	}, nil
}

// extractRange pulls lines [startLine, endLine] (1-indexed) from raw file bytes.
func extractRange(content []byte, startLine, endLine int) string {
	lines := strings.Split(string(content), "\n")
	if startLine < 1 {
		startLine = 1
	}
	if endLine > len(lines) {
		endLine = len(lines)
	}
	return strings.Join(lines[startLine-1:endLine], "\n")
}

// providerFromResourceType infers the provider prefix, e.g. "aws" from "aws_s3_bucket".
func providerFromResourceType(resourceType string) string {
	parts := strings.SplitN(resourceType, "_", 2)
	if len(parts) > 0 {
		return parts[0]
	}
	return ""
}

// attrStringValue extracts the string value of a named attribute from a block.
func attrStringValue(block *hclsyntax.Block, attrName string) string {
	attr, ok := block.Body.Attributes[attrName]
	if !ok {
		return ""
	}
	val, diags := attr.Expr.Value(nil)
	if diags.HasErrors() {
		return ""
	}
	if val.Type() == cty.String {
		return val.AsString()
	}
	return ""
}

// extractReferences finds Terraform reference expressions in source text.
// Covers: var.X, module.X.Y, local.X, data.TYPE.NAME, each.key, each.value
func extractReferences(content string) []string {
	patterns := []string{"var.", "module.", "local.", "data.", "each."}
	seen := map[string]bool{}
	var refs []string

	for _, pat := range patterns {
		idx := 0
		for {
			pos := strings.Index(content[idx:], pat)
			if pos < 0 {
				break
			}
			pos += idx
			end := pos + len(pat)
			// Consume the identifier after the prefix.
			for end < len(content) && isIdentChar(content[end]) {
				end++
			}
			// For module refs, also consume .attribute
			if pat == "module." && end < len(content) && content[end] == '.' {
				end++ // consume dot
				for end < len(content) && isIdentChar(content[end]) {
					end++
				}
			}
			ref := content[pos:end]
			if !seen[ref] {
				seen[ref] = true
				refs = append(refs, ref)
			}
			idx = pos + 1
		}
	}
	return refs
}

func isIdentChar(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') ||
		(c >= '0' && c <= '9') || c == '_' || c == '-'
}

func chunkID(filePath string, startLine int, content string) string {
	h := sha256.Sum256([]byte(fmt.Sprintf("%s:%d:%s", filePath, startLine, content)))
	return fmt.Sprintf("%x", h)[:16]
}

func chunkSymbolID(chunk types.Chunk) string {
	h := sha256.Sum256([]byte(chunk.ID + ":def:" + chunk.Name))
	return fmt.Sprintf("%x", h)[:16]
}

func refSymbolID(chunk types.Chunk, ref string) string {
	h := sha256.Sum256([]byte(chunk.ID + ":ref:" + ref))
	return fmt.Sprintf("%x", h)[:16]
}
