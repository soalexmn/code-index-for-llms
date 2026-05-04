# code-index-for-llms

Semantic code indexing for LLMs. Parses your codebase into structured chunks, stores them in a local SQLite database, and exposes them via an [MCP](https://modelcontextprotocol.io) server so Claude (and other LLMs) can search and retrieve relevant code with context.

- **Single binary** - no runtime dependencies, no servers, no Docker
- **BM25 + vector hybrid search** - keyword search out of the box, vector search opt-in
- **IaC-aware** - Terraform/HCL parsed to typed resource chunks with metadata
- **Native Claude CLI plugin** - auto-injects code context into every session

---

## Benchmark

Measured on a synthetic multi-language project (20 files, Python / Go / TypeScript / Terraform) with 20 ground-truth queries across function lookup, class lookup, IaC resource lookup, and cross-language concepts.

| Approach                          | Recall@10 | MRR      | Avg context tokens | Token reduction |
| --------------------------------- | --------- | -------- | ------------------ | --------------- |
| No plugin (full codebase)         | 0.00      | 0.00     | 9,862              | -               |
| Caveman (output compression only) | 0.00      | 0.00     | 9,862              | 0%              |
| code-context-engine (CCE)         | 0.37      | 0.39     | 1,453              | 85%             |
| **code-index-for-llms (this)**    | **0.83**  | **0.72** | **801**            | **92%**         |

- **Recall@10**: fraction of relevant chunks appearing in top-10 results (higher is better)
- **MRR**: mean reciprocal rank of the first relevant result (higher is better)
- **Token reduction**: `(baseline_tokens − runner_tokens) / baseline_tokens`
- Baseline recall is 0 because it returns whole files, not named chunks - the LLM sees all code but the retrieval system cannot match function-level ground truth
- CCE recall is lower due to coarser chunking (whole classes / whole files vs individual functions and resources)
- Reproduce: `go test ./tests/comparison/... -v -timeout 15m`

---

## Quick start

```bash
# 1. Build
go build -o bin/code-index ./cmd/code-index

# 2. Index your project
./bin/code-index index /path/to/your/project

# 3. Search
./bin/code-index search --root /path/to/your/project "S3 bucket encryption"

# 4. Start MCP server (Claude connects to this)
./bin/code-index mcp serve --root /path/to/your/project
```

---

## Language support

| Language        | Parser        | Chunk types                                 |
| --------------- | ------------- | ------------------------------------------- |
| Go              | regex AST     | FUNCTION, METHOD, CLASS (struct), INTERFACE |
| Python          | regex AST     | FUNCTION, METHOD, CLASS                     |
| Terraform / HCL | hclsyntax AST | RESOURCE, MODULE, VARIABLE, BLOCK           |
| Everything else | line-based    | FILE                                        |

Adding a language: implement `LanguageParser` in `internal/parser/<lang>/parser.go`, register with one line in `buildRegistry()`.

---

## MCP tools

| Tool                   | Description                                             |
| ---------------------- | ------------------------------------------------------- |
| `index_project`        | Full index - parse all files, extract chunks + symbols  |
| `refresh_index`        | Incremental update - re-index only changed files        |
| `get_index_status`     | Index health, stats, detected languages                 |
| `search_code`          | Hybrid BM25 + vector search                             |
| `get_chunk`            | Retrieve full chunk content by ID                       |
| `get_file_symbols`     | List symbols defined in a file                          |
| `find_references`      | Find all definitions/references for a symbol name       |
| `query_resources`      | IaC-specific: filter by resource type, provider, module |
| `get_dependency_graph` | Dependency graph (nodes + edges) for a chunk            |
| `assemble_context`     | Token-budgeted context assembly from search results     |
| `list_languages`       | Available parsers and detected languages                |

---

## Configuration

Copy `.code-index.yaml.example` to `.code-index.yaml` in your project root:

```yaml
project:
  name: my-project

index:
  exclude:
    - ".git"
    - "node_modules"
    - "vendor"
    - ".terraform"
  max_file_size_kb: 500

embedding:
  provider: "none" # "none" (BM25 only), "openai", or "ollama"
  # model: "text-embedding-3-small"
  # api_key_env: "OPENAI_API_KEY"

storage:
  path: ".code-index/index.db"
```

Default provider is `none` - BM25-only search with zero configuration. Enable vector search by setting `provider: openai` or `provider: ollama`.

---

## Claude CLI plugin

Install as a Claude CLI plugin for automatic code context injection:

```jsonc
// ~/.claude/settings.json - register the marketplace
{
  "extraKnownMarketplaces": {
    "code-index": {
      "source": { "source": "github", "repo": "YOUR_ORG/code-index-for-llms" },
    },
  },
}
```

```
/install code-index@code-index
```

After install:

- **SessionStart** - detects existing index, injects stats or suggests `/index`
- **UserPromptSubmit** - auto-prepends top-3 search results as `<code_context>` before each prompt
- **`/index`** - index the current project
- **`/search`** - search indexed code
- **`/refresh`** - re-index changed files
- **`/status`** - show index stats

Disable auto-context: `CODE_INDEX_DISABLE_AUTO_CONTEXT=1`

---

## Architecture

See [`docs/architecture.md`](docs/architecture.md) for the full design.

```
internal/
  parser/           # Language parsers (implement LanguageParser interface)
    terraform/      # HCL/Terraform - hclsyntax AST
    python/         # Python - regex-based AST
    golang/         # Go - regex-based AST
    generic/        # Fallback - sliding window line chunker
  storage/          # SQLite store - FTS5 BM25 + sqlite-vec (planned)
  indexer/          # Walker, chunker, embedder
  mcp/              # MCP stdio server + tool handlers
  config/           # .code-index.yaml loader
pkg/types/          # Shared types: Chunk, Symbol, SearchResult, Project
cmd/code-index/     # CLI entrypoint
```

---

## Building for distribution

```bash
# All platforms
bash scripts/build-all.sh

# Single target
GOOS=linux GOARCH=amd64 go build -o bin/code-index-linux-amd64 ./cmd/code-index
```

No CGO required - pure Go SQLite via `modernc.org/sqlite`.
