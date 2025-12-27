#!/bin/sh
set -eu

VERSION="${1:-}"
if [ -z "$VERSION" ]; then
  echo "usage: $0 <version>" >&2
  exit 1
fi

OUTDIR="./dist/${VERSION}"
BINDIR="${OUTDIR}/bin"
mkdir -p "$BINDIR"

# Embed version into the binary (requires: var version in package main)
LDFLAGS="-s -w -X main.version=${VERSION}"

# Build versioned binaries into dist/<version>/bin/
BIN_AMD64_VER="aim-${VERSION}-linux-amd64"
BIN_ARM64_VER="aim-${VERSION}-linux-arm64"

OUT_AMD64_VER="${BINDIR}/${BIN_AMD64_VER}"
OUT_ARM64_VER="${BINDIR}/${BIN_ARM64_VER}"

GOOS=linux GOARCH=amd64 CGO_ENABLED=0 \
  go build -trimpath -ldflags "$LDFLAGS" -o "$OUT_AMD64_VER" ./cmd/aim

GOOS=linux GOARCH=arm64 CGO_ENABLED=0 \
  go build -trimpath -ldflags "$LDFLAGS" -o "$OUT_ARM64_VER" ./cmd/aim

# Create stable-named tarballs in dist/<version>/
TAR_AMD64="aim-linux-amd64.tar.gz"
TAR_ARM64="aim-linux-arm64.tar.gz"

tar -C "$BINDIR" -czf "${OUTDIR}/${TAR_AMD64}" "$BIN_AMD64_VER"
tar -C "$BINDIR" -czf "${OUTDIR}/${TAR_ARM64}" "$BIN_ARM64_VER"

# Checksums for what users download (the tarballs)
(
  cd "$OUTDIR"
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$TAR_AMD64" "$TAR_ARM64" > checksums.txt
  elif command -v shasum >/dev/null 2>&1; then
    shasum -a 256 "$TAR_AMD64" "$TAR_ARM64" > checksums.txt
  else
    echo "error: need sha256sum (coreutils) or shasum" >&2
    exit 1
  fi
)

echo "artifacts in: $OUTDIR"
echo "binaries in: $BINDIR"
ls -1 "$OUTDIR"
