#!/usr/bin/env node
/**
 * UserPromptSubmit hook: detects code-related intent in the user's prompt,
 * runs a quick search, and prepends the top results as <code_context>.
 *
 * Skip entirely if:
 *   - CODE_INDEX_DISABLE_AUTO_CONTEXT=1
 *   - No index exists
 *   - Prompt does not look code-related
 */

const fs = require('fs');
const path = require('path');
const { execSync } = require('child_process');

if (process.env.CODE_INDEX_DISABLE_AUTO_CONTEXT === '1') process.exit(0);

const workspaceRoot = process.env.CODE_INDEX_ROOT || process.env.WORKSPACE_FOLDER || process.cwd();
const indexPath = path.join(workspaceRoot, '.code-index', 'index.db');

if (!fs.existsSync(indexPath)) process.exit(0);

// Read the user prompt from stdin (JSON with `prompt` field).
let input = '';
process.stdin.on('data', d => { input += d; });
process.stdin.on('end', () => {
  try {
    const { prompt } = JSON.parse(input);
    if (!isCodeRelated(prompt)) process.exit(0);

    const results = searchIndex(prompt, workspaceRoot);
    if (!results || results.length === 0) process.exit(0);

    const contextBlock = formatContext(results);
    process.stdout.write(contextBlock);
  } catch (_) {
    // Non-fatal - never block the user's prompt.
  }
  process.exit(0);
});

/**
 * Heuristic: does this prompt look like a code/infra question?
 * Avoids unnecessary searches for conversational messages.
 */
function isCodeRelated(prompt) {
  if (!prompt || prompt.length < 10) return false;

  const codeKeywords = [
    'function', 'class', 'method', 'variable', 'module', 'import',
    'resource', 'provider', 'terraform', 'aws_', 'azurerm_', 'google_',
    'how', 'why', 'where', 'what', 'show me', 'find', 'search',
    'bug', 'error', 'fix', 'implement', 'add', 'refactor',
    '.tf', '.py', '.go', '.ts', '.rs',
    'def ', 'func ', 'const ', 'let ', 'var ',
  ];

  const lower = prompt.toLowerCase();
  return codeKeywords.some(kw => lower.includes(kw));
}

function searchIndex(query, root) {
  try {
    const pluginRoot = process.env.CLAUDE_PLUGIN_ROOT || path.join(__dirname, '..');
    const binary = getBinaryPath(pluginRoot);

    const req = JSON.stringify({
      jsonrpc: '2.0', id: 1,
      method: 'tools/call',
      params: {
        name: 'search_code',
        arguments: { query, root_path: root, limit: 3 }
      }
    }) + '\n';

    const out = execSync(`"${binary}" mcp serve`, {
      input: req,
      timeout: 4000,
      encoding: 'utf8'
    });

    const lines = out.trim().split('\n').filter(Boolean);
    // Find the tools/call response (skip initialize response if present).
    for (const line of lines.reverse()) {
      try {
        const resp = JSON.parse(line);
        if (resp.result?.content?.[0]?.text) {
          const data = JSON.parse(resp.result.content[0].text);
          return data.results || [];
        }
      } catch (_) {}
    }
  } catch (_) {}
  return [];
}

function formatContext(results) {
  const blocks = results.map(r => {
    const chunk = r.chunk || r;
    const loc = `${chunk.file_path || chunk.FilePath}:${chunk.start_line || chunk.StartLine}-${chunk.end_line || chunk.EndLine}`;
    return `// ${loc} [${chunk.language || chunk.Language || 'unknown'}]\n${chunk.content || chunk.Content}`;
  });

  return [
    '<code_context>',
    'Relevant code found in this project:',
    '',
    blocks.join('\n\n---\n\n'),
    '</code_context>',
    ''
  ].join('\n');
}

function getBinaryPath(pluginRoot) {
  const candidates = [
    path.join(pluginRoot, 'bin', 'code-index'),
    path.join(pluginRoot, 'bin', 'code-index.exe'),
    'code-index',
  ];
  for (const c of candidates) {
    try { fs.accessSync(c, fs.constants.X_OK); return c; } catch (_) {}
  }
  throw new Error('code-index binary not found');
}
