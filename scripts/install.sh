#!/usr/bin/env bash
# install.sh: Download the correct code-index binary for the current platform
# and place it at $CLAUDE_PLUGIN_ROOT/bin/code-index.
#
# Called automatically by Claude Code after plugin installation.
# Can also be run manually: bash scripts/install.sh

set -euo pipefail

REPO="YOUR_ORG/code-index-for-llms"
VERSION="${CODE_INDEX_VERSION:-latest}"
PLUGIN_ROOT="${CLAUDE_PLUGIN_ROOT:-$(cd "$(dirname "$0")/.." && pwd)}"
BIN_DIR="$PLUGIN_ROOT/bin"

mkdir -p "$BIN_DIR"

# Detect OS and architecture.
OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
ARCH="$(uname -m)"

case "$ARCH" in
  x86_64)  ARCH="amd64" ;;
  arm64|aarch64) ARCH="arm64" ;;
  *) echo "Unsupported architecture: $ARCH" >&2; exit 1 ;;
esac

case "$OS" in
  linux)   EXT="" ;;
  darwin)  EXT="" ;;
  mingw*|cygwin*|msys*) OS="windows"; EXT=".exe" ;;
  *) echo "Unsupported OS: $OS" >&2; exit 1 ;;
esac

BINARY_NAME="code-index-${OS}-${ARCH}${EXT}"
TARGET="$BIN_DIR/code-index${EXT}"

if [ "$VERSION" = "latest" ]; then
  DOWNLOAD_URL="https://github.com/${REPO}/releases/latest/download/${BINARY_NAME}"
else
  DOWNLOAD_URL="https://github.com/${REPO}/releases/download/${VERSION}/${BINARY_NAME}"
fi

echo "Downloading code-index binary..."
echo "  URL: $DOWNLOAD_URL"
echo "  Destination: $TARGET"

if command -v curl >/dev/null 2>&1; then
  curl -fsSL "$DOWNLOAD_URL" -o "$TARGET"
elif command -v wget >/dev/null 2>&1; then
  wget -q "$DOWNLOAD_URL" -O "$TARGET"
else
  echo "Error: curl or wget required for installation." >&2
  exit 1
fi

chmod +x "$TARGET"
echo "code-index installed at $TARGET"
echo ""
echo "Open a new Claude Code session and run /index to index your project."
