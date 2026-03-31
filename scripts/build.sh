#!/bin/sh
set -eu

TAG_VERSION="${1:-}"

if [ -z "$TAG_VERSION" ]; then
  echo "usage: $0 <version>" >&2
  exit 1
fi

if ! command -v goreleaser >/dev/null 2>&1; then
  echo "error: goreleaser must be installed to use $0" >&2
  exit 1
fi

GORELEASER_CURRENT_TAG="$TAG_VERSION" \
  goreleaser release --clean --skip=publish,validate
