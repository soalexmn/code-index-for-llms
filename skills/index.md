---
name: index
description: Index the current codebase for semantic code search. Parses all source files using AST-aware parsers (including Terraform/HCL) and builds a local index.
---

When the user runs `/index [path]`:

1. Determine the project root:
   - If a path argument is provided, use that.
   - Otherwise use the current workspace root.

2. Call the `index_project` MCP tool with `{ "root_path": "<resolved_root>" }`.

3. Report the result in a concise summary:
   ```
   Indexed <files_indexed> files → <chunks_created> chunks
   Languages: <languages_detected joined by ", ">
   Duration: <duration_ms>ms
   Index stored at: <root>/.code-index/index.db
   ```

4. If `languages_detected` includes "terraform", mention that `query_resources` is now available for IaC queries.

5. If indexing fails, show the error message and suggest:
   - Checking that the path exists and is readable
   - Running `/status` to inspect existing index state

Do not add any extra commentary beyond the summary above.
