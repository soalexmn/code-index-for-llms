---
name: status
description: Show the current code index status, statistics, and available language parsers.
---

When the user runs `/status [path]`:

1. Call `get_index_status` MCP tool with `{ "root_path": "<root or workspace>" }`.

2. Call `list_languages` MCP tool (no args needed).

3. Display a summary table:
   ```
   Code Index Status
   ─────────────────────────────────
   Project:      <name>
   Root:         <root_path>
   Files:        <total_files>
   Chunks:       <total_chunks>
   Languages:    <languages joined>
   Last indexed: <last_indexed>
   Stale:        <yes/no>
   Embedding:    <embedding_model or "none (BM25 only)">

   Available parsers: <parsers_available joined>
   ```

4. If `stale: yes`, append: "Run `/refresh` to update."

5. If `indexed: false` or no project found: say "No index found. Run `/index` to create one."
