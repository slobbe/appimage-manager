#!/usr/bin/env bash
set -euo pipefail

if [[ $# -ne 1 ]]; then
  echo 'usage: bash scripts/validate-release-tag.sh <release-tag>' >&2
  exit 1
fi

tag=$1
semver='^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-((0|[1-9][0-9]*|[A-Za-z-][0-9A-Za-z-]*)(\.(0|[1-9][0-9]*|[A-Za-z-][0-9A-Za-z-]*))*))?$'

if [[ ! "$tag" =~ $semver ]]; then
  echo "Release tag $tag must match vX.Y.Z or vX.Y.Z-<prerelease>." >&2
  exit 1
fi

if ! sha=$(git rev-parse --verify "refs/tags/$tag^{commit}" 2>/dev/null); then
  echo "Release tag $tag must already exist and point to a commit." >&2
  exit 1
fi

if git merge-base --is-ancestor "$sha" refs/remotes/origin/main 2>/dev/null; then
  printf '%s\n' "$sha"
  exit 0
fi

if [[ "$tag" == *-* ]]; then
  version=${tag#v}
  branch="refs/remotes/origin/release/${version%%-*}"
  if git merge-base --is-ancestor "$sha" "$branch" 2>/dev/null; then
    printf '%s\n' "$sha"
    exit 0
  fi
  echo "Prerelease $tag must reference a commit on main or ${branch#refs/remotes/origin/}." >&2
else
  echo "Stable release $tag must reference a commit on main." >&2
fi
exit 1
