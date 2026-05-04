---
name: search
description: Search the indexed codebase using natural language. Returns the most relevant code chunks ranked by hybrid BM25+vector scoring.
---

When the user runs `/search <query>` or asks you to search the code index:

1. Call `search_code` MCP tool:
   ```json
   { "query": "<user query>", "limit": 5 }
   ```

2. For each result, display:
   ```
   📄 <file_path>:<start_line>-<end_line>  [<language>]  score: <score>
   ───
   <content (first 20 lines if long)>
   ```

3. If the query looks like an IaC/infrastructure question (mentions "resource", "provider", "aws", "azurerm", "google", etc.), also call `query_resources` with the relevant `resource_type` or `provider` extracted from the query, and show those results in a separate "Infrastructure Resources" section.

4. If zero results: suggest trying a broader query or running `/refresh` if the index might be stale.

5. Offer to expand any result with full context: "Type `/search --expand <chunk_id>` to see the full chunk."

Keep responses concise. Do not re-explain what the tool does.
