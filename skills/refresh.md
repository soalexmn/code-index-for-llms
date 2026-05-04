---
name: refresh
description: Incrementally update the code index. Only re-indexes files that have changed since the last index run.
---

When the user runs `/refresh [path]`:

1. Determine root path (argument or workspace root).

2. Call `refresh_index` MCP tool:
   ```json
   { "root_path": "<root>" }
   ```

3. Report result:
   ```
   Refreshed index: <files_updated> updated, <files_removed> removed
   ```

4. If `files_updated == 0 && files_removed == 0`: say "Index is up to date."

5. If an error occurs mentioning "not indexed yet": suggest running `/index` first.

Do not add extra commentary.
