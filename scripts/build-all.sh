#!/usr/bin/env bash
# build-all.sh: Cross-compile code-index for all supported platforms.
# Outputs binaries to bin/ directory for GitHub Releases upload.

set -euo pipefail

VERSION="${1:-dev}"
PACKAGE="github.com/code-index-for-llms/code-index/cmd/code-index"
OUT_DIR="bin"

mkdir -p "$OUT_DIR"

TARGETS=(
  "linux/amd64"
  "linux/arm64"
  "darwin/amd64"
  "darwin/arm64"
  "windows/amd64"
)

for target in "${TARGETS[@]}"; do
  OS="${target%/*}"
  ARCH="${target#*/}"
  EXT=""
  [ "$OS" = "windows" ] && EXT=".exe"

  NAME="code-index-${OS}-${ARCH}${EXT}"
  echo "Building $NAME..."

  CGO_ENABLED=1 GOOS="$OS" GOARCH="$ARCH" go build \
    -ldflags="-X main.version=${VERSION}" \
    -o "${OUT_DIR}/${NAME}" \
    "$PACKAGE"
done

echo ""
echo "Binaries in ${OUT_DIR}/:"
ls -lh "${OUT_DIR}/"
