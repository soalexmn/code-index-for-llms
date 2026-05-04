# Architecture: code-index-for-llms

## Context

Code indexing + context retrieval tool for LLMs - inspired by [code-context-engine](https://github.com/elara-labs/code-context-engine) with three key additions:

1. Go-based (single static binary, no runtime deps)
2. Native IaC AST support (Terraform/HCL first, Bicep/Pulumi/Ansible roadmap)
3. Native Claude CLI plugin install (same pattern as caveman - GitHub marketplace, hooks, skills)

CCE achieves 94% token reduction via local semantic indexing. This tool targets the same outcome with better distribution and infrastructure-aware semantics.

---

## Primary Language: Go

**Why Go over Python/C#:**

- Single self-contained binary → zero install friction
- Fast cold-start (critical: MCP server spawned per session)
- Excellent go-tree-sitter bindings
- Cross-compile from any platform (`GOOS=windows GOARCH=amd64`)
- Plugin hooks still Node.js (matches caveman, what Claude CLI expects)

---

## Component Diagram

```
┌─────────────────────────────────────────────────────────────────────┐
│                        Claude Code CLI                              │
│                                                                     │
│  ┌─────────────────────────────────────────────────────────────┐   │
│  │                  Plugin Layer (JS/Node)                      │   │
│  │                                                              │   │
│  │  hooks/SessionStart.js    hooks/UserPromptSubmit.js         │   │
│  │  skills/index.md          skills/search.md                  │   │
│  │  .claude-plugin/plugin.json                                  │   │
│  └────────────────────────┬─────────────────────────────────────┘   │
│                           │ spawns / manages                        │
│  ┌────────────────────────▼─────────────────────────────────────┐   │
│  │                 MCP Server (Go binary)                        │   │
│  │                                                               │   │
│  │  index_project      refresh_index     get_index_status       │   │
│  │  search_code        get_chunk         get_file_symbols       │   │
│  │  find_references    query_resources   get_dependency_graph   │   │
│  │  assemble_context   list_languages                           │   │
│  └────────┬─────────────────────────────────────┬───────────────┘   │
└───────────│─────────────────────────────────────│───────────────────┘
            │                                     │
            ▼                                     ▼
┌───────────────────────┐             ┌───────────────────────────┐
│   Indexer Engine      │             │   Storage Layer            │
│                       │             │                            │
│  File Walker          │             │  SQLite (main DB)          │
│  + FSNotify Watcher   │             │  - projects table          │
│  Parser Registry      │             │  - files table             │
│  Chunker              │             │  - chunks table            │
│  Embedder             │             │  - symbols table           │
└───────────┬───────────┘             │  sqlite-vec (vectors)      │
            │                         │  FTS5 (BM25)               │
            ▼                         └───────────────────────────┘
┌───────────────────────────────────────────────────────────────────┐
│                    Parser Plugin Interface                          │
│                                                                    │
│  Built-in: Python, Go, JS/TS, Rust, Java, Terraform/HCL          │
│  Extensible: Bicep, Pulumi, Ansible, Helm                         │
└───────────────────────────────────────────────────────────────────┘
```

---

## Repository Structure

```
code-index-for-llms/
├── .claude-plugin/
│   └── plugin.json              # Hook defs, skill paths, MCP server config
├── hooks/                       # Node.js hook scripts
│   ├── SessionStart.js          # Detect index, inject context or suggest /index
│   └── UserPromptSubmit.js      # Auto-prepend top-3 results as <code_context>
├── skills/                      # Slash command definitions
│   ├── index.md                 # /index
│   ├── search.md                # /search
│   ├── refresh.md               # /refresh
│   └── status.md                # /status
├── cmd/code-index/main.go       # Binary entrypoint
├── internal/
│   ├── parser/
│   │   ├── interface.go         # LanguageParser interface
│   │   ├── registry.go          # Compile-time registration + dispatch
│   │   ├── python/parser.go
│   │   ├── golang/parser.go
│   │   ├── typescript/parser.go
│   │   ├── rust/parser.go
│   │   ├── java/parser.go
│   │   ├── terraform/parser.go  # HCL + Terraform-specific semantics
│   │   └── generic/parser.go    # Fallback line-based chunker
│   ├── indexer/
│   │   ├── walker.go            # File system traversal
│   │   ├── watcher.go           # fsnotify incremental updates
│   │   ├── chunker.go           # Orchestrates parser → chunks
│   │   └── embedder.go          # Embedder interface + impls
│   ├── storage/
│   │   ├── store.go             # Storage interface
│   │   ├── sqlite.go            # sqlite + sqlite-vec + FTS5
│   │   ├── migrations/          # SQL migration files
│   │   └── query.go             # Hybrid search (RRF fusion)
│   ├── mcp/
│   │   ├── server.go            # MCP server (stdio transport)
│   │   ├── tools.go             # Tool registration
│   │   └── handlers/            # Per-tool handler files
│   └── config/
│       ├── config.go            # Config struct + defaults
│       └── loader.go            # .code-index.yaml + env var overrides
├── pkg/types/types.go           # Shared types: Chunk, Symbol, SearchResult, Project
├── scripts/
│   ├── install.sh               # Download platform binary from GitHub Releases
│   └── build-all.sh             # Cross-compile script
├── docs/
│   └── architecture.md          # This file
│   └── adding-a-parser.md       # Guide for contributors
└── .code-index.yaml.example
```

---

## Key Data Models

### Chunk

```go
type Chunk struct {
    ID            string
    FileID        string
    FilePath      string            // Relative to project root
    Language      string
    ChunkType     ChunkType
    Name          string            // "aws_s3_bucket.main", "MyClass", "parse_config"
    Content       string
    StartLine     int
    EndLine       int
    ParentChunkID string            // For nested structures (method inside class)
    Embedding     []float32
    Metadata      map[string]string // TF: resource_type, provider, resource_name
    ContentHash   string
    IndexedAt     time.Time
}
```

### ChunkType

```
FUNCTION | CLASS | METHOD | INTERFACE | VARIABLE | RESOURCE | MODULE | BLOCK | IMPORT | COMMENT | FILE
```

### Symbol

```go
type Symbol struct {
    ID      string
    ChunkID string
    Name    string
    Kind    SymbolKind   // DEFINITION | REFERENCE | IMPORT
    FileID  string
    Line    int
}
```

### SearchResult

```go
type SearchResult struct {
    Chunk       Chunk
    Score       float64  // RRF combined
    VectorScore float64
    BM25Score   float64
}
```

---

## Language Parser Interface

```go
type LanguageParser interface {
    Language() string
    Extensions() []string
    CanParse(filePath string, content []byte) bool
    Parse(filePath string, content []byte) ([]Chunk, error)
    ExtractSymbols(chunks []Chunk) ([]Symbol, error)
    SupportedChunkTypes() []ChunkType
}
```

Registration is compile-time only (no Go plugin ABI). Add one line to `registry.go`.

---

## Terraform Parser: Special Semantics

Each HCL block maps to ChunkType + structured Metadata:

| HCL Block                | ChunkType           | Metadata keys                          |
| ------------------------ | ------------------- | -------------------------------------- |
| `resource "TYPE" "NAME"` | RESOURCE            | resource_type, resource_name, provider |
| `module "NAME"`          | MODULE              | module_name, source                    |
| `variable "NAME"`        | VARIABLE            | var_name, type, default                |
| `output "NAME"`          | VARIABLE            | output_name                            |
| `locals {}`              | VARIABLE (per attr) | local_name                             |
| `provider {}`            | BLOCK               | provider_name                          |
| `terraform {}`           | BLOCK               | -                                      |

Extracts symbol cross-refs (`var.foo`, `module.bar.output`, resource refs) → dependency graph.

---

## MCP Tools (11 total)

| Tool                   | Purpose                                                                  |
| ---------------------- | ------------------------------------------------------------------------ |
| `index_project`        | Full index. Returns: files/chunks/languages/duration                     |
| `refresh_index`        | Incremental: diff by content hash or explicit file list                  |
| `get_index_status`     | Health, staleness, stats                                                 |
| `search_code`          | Hybrid vector+BM25. Args: query, language?, chunk_types?, hybrid_weight? |
| `get_chunk`            | Full content + surrounding context lines                                 |
| `get_file_symbols`     | Symbol table for a file                                                  |
| `find_references`      | Definitions + references for a symbol name                               |
| `query_resources`      | **IaC-specific**: filter by resource_type, provider, module              |
| `get_dependency_graph` | Nodes + edges, configurable depth                                        |
| `assemble_context`     | Meta-tool: search + related + deps → token-budgeted context blocks       |
| `list_languages`       | Available parsers + detected languages in project                        |

---

## Claude CLI Plugin

### `.claude-plugin/plugin.json`

```json
{
  "name": "code-index-for-llms",
  "hooks": {
    "SessionStart": [
      { "command": "node ${CLAUDE_PLUGIN_ROOT}/hooks/SessionStart.js" }
    ],
    "UserPromptSubmit": [
      { "command": "node ${CLAUDE_PLUGIN_ROOT}/hooks/UserPromptSubmit.js" }
    ]
  },
  "mcpServers": {
    "code-index": {
      "type": "stdio",
      "command": "${CLAUDE_PLUGIN_ROOT}/bin/code-index",
      "args": ["mcp", "serve"]
    }
  }
}
```

**SessionStart.js**: Detect `.code-index/index.db` → if found, inject status summary. If missing, suggest `/index`.

**UserPromptSubmit.js**: Detect code-intent keywords → call `search_code` → prepend top-3 as `<code_context>`. Skip if `CODE_INDEX_DISABLE_AUTO_CONTEXT=1`.

---

## Storage Layer

Single `.code-index/index.db` file. No server process.

**Tables:**

- `projects` - one row per root path
- `files` - content_hash for incremental diff
- `chunks` - full chunk data + metadata_json
- `chunk_embeddings` - sqlite-vec virtual table (`FLOAT[N]`)
- `chunk_fts` - FTS5 virtual table for BM25
- `symbols` - cross-reference index

**Hybrid Search (RRF):**

1. Embed query
2. Parallel: vector `MATCH` + FTS5 `MATCH` (each returns top 2N)
3. Reciprocal Rank Fusion: `score = 1/(60+rank_v) + 1/(60+rank_bm25)`
4. Return top N by RRF score

---

## Embedding Providers

```go
type Embedder interface {
    Embed(texts []string) ([][]float32, error)
    Dimensions() int
    ModelID() string
}
```

Built-in: OpenAI, Anthropic (voyage-code-2), Ollama, **NoOp** (default - BM25 only, zero cost).

---

## Installation Flow

```bash
# 1. Register marketplace in settings.json
"extraKnownMarketplaces": {
  "code-index": { "source": { "source": "github", "repo": "YOUR_ORG/code-index-for-llms" }}
}

# 2. Install
/install code-index@code-index
# → clones repo, registers hooks/skills, runs scripts/install.sh (downloads binary)

# 3. First session: SessionStart fires → suggests /index
# 4. /index → MCP call → index built → auto-context active
```

---

## IaC Language Roadmap

| Tier      | Languages                   | Notes                                  |
| --------- | --------------------------- | -------------------------------------- |
| v1.0      | Terraform/HCL, OpenTofu     | Same parser                            |
| v1.1      | Bicep                       | Custom parser (no tree-sitter grammar) |
| v1.2      | Pulumi YAML, Ansible, Helm  | YAML + task-aware heuristics           |
| Community | CDK (TS/Python), Crossplane | Reuse existing language parsers        |

---

## Implementation Order

1. `pkg/types/types.go` - shared models, everything imports this
2. `internal/parser/interface.go` - LanguageParser contract
3. `internal/parser/registry.go` - dispatch layer
4. `internal/storage/sqlite.go` - schema + hybrid search
5. `internal/parser/terraform/parser.go` - IaC differentiator
6. `internal/parser/python/parser.go` - tree-sitter based
7. `internal/parser/generic/parser.go` - fallback
8. `internal/indexer/` - walker, chunker, embedder
9. `internal/mcp/` - server + handlers
10. `cmd/code-index/main.go` - CLI entrypoint
11. `.claude-plugin/` + `hooks/` + `skills/` - install UX

---

## Verification

- `go test ./...` - unit tests per parser with fixture files
- `code-index index .` then `code-index search "S3 bucket resource"` - smoke test
- `code-index mcp serve` in stdio mode + call tools via JSON-RPC manually
- Install plugin in fresh Claude Code session, run `/index`, verify `<code_context>` injection
- Terraform test: index `.tf` file, call `query_resources` with `resource_type=aws_s3_bucket`
