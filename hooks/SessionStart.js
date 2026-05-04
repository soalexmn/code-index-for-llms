#!/usr/bin/env node
/**
 * SessionStart hook: detects whether the current workspace has a code index,
 * injects a status summary into session context, or suggests running /index.
 */

const fs = require('fs');
const path = require('path');
const { execSync } = require('child_process');

const workspaceRoot = process.env.CODE_INDEX_ROOT || process.env.WORKSPACE_FOLDER || process.cwd();
const disabled = process.env.CODE_INDEX_DISABLE === '1';

if (disabled) process.exit(0);

const indexPath = path.join(workspaceRoot, '.code-index', 'index.db');
const hasIndex = fs.existsSync(indexPath);

if (!hasIndex) {
  // Emit a suggestion message via stdout (Claude Code injects this as context).
  const msg = [
    '<system-context>',
    'code-index: No code index found for this workspace.',
    `Workspace: ${workspaceRoot}`,
    'Run /index to create a semantic index for faster, cheaper code search.',
    'Or use /status to check index details.',
    '</system-context>'
  ].join('\n');
  process.stdout.write(msg + '\n');
  process.exit(0);
}

// Index exists - query status and emit summary.
try {
  const pluginRoot = process.env.CLAUDE_PLUGIN_ROOT || __dirname.replace('/hooks', '');
  const binary = getBinaryPath(pluginRoot);

  const statusJson = execSync(
    `"${binary}" mcp serve`,
    {
      input: JSON.stringify({
        jsonrpc: '2.0',
        id: 1,
        method: 'tools/call',
        params: {
          name: 'get_index_status',
          arguments: { root_path: workspaceRoot }
        }
      }) + '\n',
      timeout: 5000,
      encoding: 'utf8'
    }
  );

  // Parse the last line of output (MCP responses are newline-delimited).
  const lines = statusJson.trim().split('\n').filter(Boolean);
  const lastLine = lines[lines.length - 1];
  const response = JSON.parse(lastLine);

  if (response.result) {
    const contentText = response.result.content?.[0]?.text;
    const status = contentText ? JSON.parse(contentText) : null;

    if (status && status.total_files > 0) {
      const staleWarning = status.is_stale ? ' [STALE - run /refresh]' : '';
      const msg = [
        '<system-context>',
        `code-index: ${status.total_files} files, ${status.total_chunks} chunks indexed${staleWarning}`,
        `Languages: ${(status.languages || []).join(', ') || 'none detected'}`,
        `Last indexed: ${status.last_indexed || 'unknown'}`,
        'Use /search <query> to search, /refresh to update, /status for details.',
        '</system-context>'
      ].join('\n');
      process.stdout.write(msg + '\n');
    }
  }
} catch (_) {
  // Binary not installed or index unreadable - silently skip.
}

process.exit(0);

function getBinaryPath(pluginRoot) {
  const platform = process.platform;
  const arch = process.arch === 'arm64' ? 'arm64' : 'amd64';

  const candidates = [
    path.join(pluginRoot, 'bin', 'code-index'),
    path.join(pluginRoot, 'bin', 'code-index.exe'),
    path.join(pluginRoot, 'bin', `${platform}-${arch}`, 'code-index'),
    'code-index', // PATH fallback
  ];

  for (const c of candidates) {
    try {
      fs.accessSync(c, fs.constants.X_OK);
      return c;
    } catch (_) {}
  }
  throw new Error('code-index binary not found. Run the install script.');
}
