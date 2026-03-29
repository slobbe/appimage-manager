#!/bin/sh
set -eu

TAG_VERSION="${1:-}"
if [ -z "$TAG_VERSION" ]; then
  echo "usage: $0 <version>" >&2
  exit 1
fi

case "$TAG_VERSION" in
  v[0-9]*.[0-9]*.[0-9]*)
    ;;
  *)
    echo "error: version must match v<major>.<minor>.<patch>" >&2
    exit 1
    ;;
esac

RELEASE_VERSION="${TAG_VERSION#v}"
if [ -z "$RELEASE_VERSION" ] || [ "$RELEASE_VERSION" = "$TAG_VERSION" ]; then
  echo "error: failed to derive release version from tag: $TAG_VERSION" >&2
  exit 1
fi

OUTDIR="./dist/${TAG_VERSION}"
BINDIR="${OUTDIR}/bin"
MANDIR="${OUTDIR}/share/man/man1"
BASHCOMPDIR="${OUTDIR}/share/bash-completion/completions"
ZSHCOMPDIR="${OUTDIR}/share/zsh/site-functions"
FISHCOMPDIR="${OUTDIR}/share/fish/vendor_completions.d"
PKGDIR_AMD64="${OUTDIR}/pkg-amd64"
PKGDIR_ARM64="${OUTDIR}/pkg-arm64"
rm -rf "$PKGDIR_AMD64" "$PKGDIR_ARM64"
mkdir -p "$BINDIR" "$MANDIR" "$BASHCOMPDIR" "$ZSHCOMPDIR" "$FISHCOMPDIR"

# Embed version into the binary (requires: var version in package main)
LDFLAGS="-s -w -X main.version=${TAG_VERSION}"

# Build versioned binaries into dist/<version>/bin/
BIN_AMD64_VER="aim-${RELEASE_VERSION}-linux-amd64"
BIN_ARM64_VER="aim-${RELEASE_VERSION}-linux-arm64"

OUT_AMD64_VER="${BINDIR}/${BIN_AMD64_VER}"
OUT_ARM64_VER="${BINDIR}/${BIN_ARM64_VER}"
MANPAGE="${MANDIR}/aim.1"

AIM_MAN_OUTPUT="$MANPAGE" AIM_COMPLETION_DIR="$OUTDIR" go run -ldflags "-X main.version=${TAG_VERSION}" -tags docgen ./cmd/aim

GOOS=linux GOARCH=amd64 CGO_ENABLED=0 \
  go build -trimpath -ldflags "$LDFLAGS" -o "$OUT_AMD64_VER" ./cmd/aim

GOOS=linux GOARCH=arm64 CGO_ENABLED=0 \
  go build -trimpath -ldflags "$LDFLAGS" -o "$OUT_ARM64_VER" ./cmd/aim

# Create versioned tarballs in dist/<version>/
TAR_AMD64="aim-${RELEASE_VERSION}-linux-amd64.tar.gz"
TAR_ARM64="aim-${RELEASE_VERSION}-linux-arm64.tar.gz"

mkdir -p "${PKGDIR_AMD64}/bin" "${PKGDIR_AMD64}/share/man/man1"
mkdir -p "${PKGDIR_ARM64}/bin" "${PKGDIR_ARM64}/share/man/man1"

cp "$OUT_AMD64_VER" "${PKGDIR_AMD64}/bin/${BIN_AMD64_VER}"
cp "$OUT_ARM64_VER" "${PKGDIR_ARM64}/bin/${BIN_ARM64_VER}"
cp -R "${OUTDIR}/share/." "${PKGDIR_AMD64}/share/"
cp -R "${OUTDIR}/share/." "${PKGDIR_ARM64}/share/"

tar -C "$PKGDIR_AMD64" -czf "${OUTDIR}/${TAR_AMD64}" .
tar -C "$PKGDIR_ARM64" -czf "${OUTDIR}/${TAR_ARM64}" .
rm -rf "$PKGDIR_AMD64" "$PKGDIR_ARM64"

assert_tar_contains() {
  archive="$1"
  path="$2"
  if ! tar -tzf "$archive" | grep -Fx -- "$path" >/dev/null 2>&1; then
    echo "error: archive $archive missing $path" >&2
    exit 1
  fi
}

assert_tar_contains "${OUTDIR}/${TAR_AMD64}" "./bin/${BIN_AMD64_VER}"
assert_tar_contains "${OUTDIR}/${TAR_AMD64}" "./share/man/man1/aim.1"
assert_tar_contains "${OUTDIR}/${TAR_AMD64}" "./share/bash-completion/completions/aim"
assert_tar_contains "${OUTDIR}/${TAR_AMD64}" "./share/zsh/site-functions/_aim"
assert_tar_contains "${OUTDIR}/${TAR_AMD64}" "./share/fish/vendor_completions.d/aim.fish"

assert_tar_contains "${OUTDIR}/${TAR_ARM64}" "./bin/${BIN_ARM64_VER}"
assert_tar_contains "${OUTDIR}/${TAR_ARM64}" "./share/man/man1/aim.1"
assert_tar_contains "${OUTDIR}/${TAR_ARM64}" "./share/bash-completion/completions/aim"
assert_tar_contains "${OUTDIR}/${TAR_ARM64}" "./share/zsh/site-functions/_aim"
assert_tar_contains "${OUTDIR}/${TAR_ARM64}" "./share/fish/vendor_completions.d/aim.fish"

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
echo "completion files in: ${OUTDIR}/share"
ls -1 "$OUTDIR"
