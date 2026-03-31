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
SHAREDIR="${OUTDIR}/share"
BINDIR="${OUTDIR}/bin"
MANDIR="${SHAREDIR}/man/man1"
BASHCOMPDIR="${SHAREDIR}/bash-completion/completions"
ZSHCOMPDIR="${SHAREDIR}/zsh/site-functions"
FISHCOMPDIR="${SHAREDIR}/fish/vendor_completions.d"
MANPAGE="${MANDIR}/aim.1"

mkdir -p "$BINDIR" "$MANDIR" "$BASHCOMPDIR" "$ZSHCOMPDIR" "$FISHCOMPDIR"

# Local release reproduction mirrors the workflow's shared-assets step.
AIM_MAN_OUTPUT="$MANPAGE" AIM_COMPLETION_DIR="$OUTDIR" \
  go run -ldflags "-X main.version=${RELEASE_VERSION}" -tags docgen ./cmd/aim

build_arch() {
  goarch="$1"
  pkgdir="${OUTDIR}/pkg-${goarch}"
  bin_name="aim-${RELEASE_VERSION}-linux-${goarch}"
  out_bin="${BINDIR}/${bin_name}"
  archive="${OUTDIR}/${bin_name}.tar.gz"

  GOOS=linux GOARCH="$goarch" CGO_ENABLED=0 \
    go build -trimpath -ldflags "-s -w -X main.version=${RELEASE_VERSION}" -o "$out_bin" ./cmd/aim

  rm -rf "$pkgdir"
  mkdir -p "${pkgdir}/bin" "${pkgdir}/share"
  cp "$out_bin" "${pkgdir}/bin/${bin_name}"
  cp -R "${SHAREDIR}/." "${pkgdir}/share/"
  tar -C "$pkgdir" -czf "$archive" .
  rm -rf "$pkgdir"
}

assert_tar_contains() {
  archive="$1"
  path="$2"
  if ! tar -tzf "$archive" | grep -Fx -- "$path" >/dev/null 2>&1; then
    echo "error: archive $archive missing $path" >&2
    exit 1
  fi
}

build_arch amd64
build_arch arm64

assert_tar_contains "${OUTDIR}/aim-${RELEASE_VERSION}-linux-amd64.tar.gz" "./bin/aim-${RELEASE_VERSION}-linux-amd64"
assert_tar_contains "${OUTDIR}/aim-${RELEASE_VERSION}-linux-amd64.tar.gz" "./share/man/man1/aim.1"
assert_tar_contains "${OUTDIR}/aim-${RELEASE_VERSION}-linux-amd64.tar.gz" "./share/bash-completion/completions/aim"
assert_tar_contains "${OUTDIR}/aim-${RELEASE_VERSION}-linux-amd64.tar.gz" "./share/zsh/site-functions/_aim"
assert_tar_contains "${OUTDIR}/aim-${RELEASE_VERSION}-linux-amd64.tar.gz" "./share/fish/vendor_completions.d/aim.fish"

assert_tar_contains "${OUTDIR}/aim-${RELEASE_VERSION}-linux-arm64.tar.gz" "./bin/aim-${RELEASE_VERSION}-linux-arm64"
assert_tar_contains "${OUTDIR}/aim-${RELEASE_VERSION}-linux-arm64.tar.gz" "./share/man/man1/aim.1"
assert_tar_contains "${OUTDIR}/aim-${RELEASE_VERSION}-linux-arm64.tar.gz" "./share/bash-completion/completions/aim"
assert_tar_contains "${OUTDIR}/aim-${RELEASE_VERSION}-linux-arm64.tar.gz" "./share/zsh/site-functions/_aim"
assert_tar_contains "${OUTDIR}/aim-${RELEASE_VERSION}-linux-arm64.tar.gz" "./share/fish/vendor_completions.d/aim.fish"

# Checksums for what users download (the tarballs)
(
  cd "$OUTDIR"
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum \
      "aim-${RELEASE_VERSION}-linux-amd64.tar.gz" \
      "aim-${RELEASE_VERSION}-linux-arm64.tar.gz" \
      > checksums.txt
  elif command -v shasum >/dev/null 2>&1; then
    shasum -a 256 \
      "aim-${RELEASE_VERSION}-linux-amd64.tar.gz" \
      "aim-${RELEASE_VERSION}-linux-arm64.tar.gz" \
      > checksums.txt
  else
    echo "error: need sha256sum (coreutils) or shasum" >&2
    exit 1
  fi
)

echo "artifacts in: $OUTDIR"
echo "binaries in: $BINDIR"
echo "man page in: $MANPAGE"
echo "completion files in: ${SHAREDIR}"
ls -1 "$OUTDIR"
