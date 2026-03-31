#!/bin/sh
set -eu

TAG_VERSION="${1:-}"
if [ -z "$TAG_VERSION" ]; then
  echo "error: usage: $0 <version>" >&2
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
MANDIR="${SHAREDIR}/man/man1"
BASHCOMPDIR="${SHAREDIR}/bash-completion/completions"
ZSHCOMPDIR="${SHAREDIR}/zsh/site-functions"
FISHCOMPDIR="${SHAREDIR}/fish/vendor_completions.d"
MANPAGE="${MANDIR}/aim.1"

rm -rf "$SHAREDIR"
mkdir -p "$MANDIR" "$BASHCOMPDIR" "$ZSHCOMPDIR" "$FISHCOMPDIR"

AIM_MAN_OUTPUT="$MANPAGE" AIM_COMPLETION_DIR="$OUTDIR" \
  go run -ldflags "-X main.version=${RELEASE_VERSION}" -tags docgen ./cmd/aim

[ -d "$SHAREDIR" ] || {
  echo "error: missing shared assets directory: $SHAREDIR" >&2
  exit 1
}
[ -f "$SHAREDIR/man/man1/aim.1" ] || {
  echo "error: missing man page in shared assets" >&2
  exit 1
}
[ -f "$SHAREDIR/bash-completion/completions/aim" ] || {
  echo "error: missing bash completion in shared assets" >&2
  exit 1
}
[ -f "$SHAREDIR/zsh/site-functions/_aim" ] || {
  echo "error: missing zsh completion in shared assets" >&2
  exit 1
}
[ -f "$SHAREDIR/fish/vendor_completions.d/aim.fish" ] || {
  echo "error: missing fish completion in shared assets" >&2
  exit 1
}

echo "prepared shared release assets in: $SHAREDIR"
