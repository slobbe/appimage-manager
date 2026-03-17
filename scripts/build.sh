#!/bin/sh
set -eu

VERSION="${1:-}"
if [ -z "$VERSION" ]; then
  echo "usage: $0 <version>" >&2
  exit 1
fi

OUTDIR="./dist/${VERSION}"
BINDIR="${OUTDIR}/bin"
MANDIR="${OUTDIR}/share/man/man1"
PKGDIR_AMD64="${OUTDIR}/pkg-amd64"
PKGDIR_ARM64="${OUTDIR}/pkg-arm64"
rm -rf "$PKGDIR_AMD64" "$PKGDIR_ARM64"
mkdir -p "$BINDIR" "$MANDIR"

# Embed version into the binary (requires: var version in package main)
LDFLAGS="-s -w -X main.version=${VERSION}"

# Build versioned binaries into dist/<version>/bin/
BIN_AMD64_VER="aim-${VERSION}-linux-amd64"
BIN_ARM64_VER="aim-${VERSION}-linux-arm64"

OUT_AMD64_VER="${BINDIR}/${BIN_AMD64_VER}"
OUT_ARM64_VER="${BINDIR}/${BIN_ARM64_VER}"
MANPAGE="${MANDIR}/aim.1"

AIM_MAN_OUTPUT="$MANPAGE" go run -ldflags "-X main.version=${VERSION}" -tags docgen ./cmd/aim

GOOS=linux GOARCH=amd64 CGO_ENABLED=0 \
  go build -trimpath -ldflags "$LDFLAGS" -o "$OUT_AMD64_VER" ./cmd/aim

GOOS=linux GOARCH=arm64 CGO_ENABLED=0 \
  go build -trimpath -ldflags "$LDFLAGS" -o "$OUT_ARM64_VER" ./cmd/aim

# Create versioned tarballs in dist/<version>/
TAR_AMD64="aim-${VERSION}-linux-amd64.tar.gz"
TAR_ARM64="aim-${VERSION}-linux-arm64.tar.gz"

mkdir -p "${PKGDIR_AMD64}/bin" "${PKGDIR_AMD64}/share/man/man1"
mkdir -p "${PKGDIR_ARM64}/bin" "${PKGDIR_ARM64}/share/man/man1"

cp "$OUT_AMD64_VER" "${PKGDIR_AMD64}/bin/${BIN_AMD64_VER}"
cp "$OUT_ARM64_VER" "${PKGDIR_ARM64}/bin/${BIN_ARM64_VER}"
cp "$MANPAGE" "${PKGDIR_AMD64}/share/man/man1/aim.1"
cp "$MANPAGE" "${PKGDIR_ARM64}/share/man/man1/aim.1"

tar -C "$PKGDIR_AMD64" -czf "${OUTDIR}/${TAR_AMD64}" .
tar -C "$PKGDIR_ARM64" -czf "${OUTDIR}/${TAR_ARM64}" .
rm -rf "$PKGDIR_AMD64" "$PKGDIR_ARM64"

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
echo "man page in: $MANPAGE"
ls -1 "$OUTDIR"
