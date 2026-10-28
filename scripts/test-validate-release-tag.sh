#!/usr/bin/env bash
set -euo pipefail

repo_root=$(CDPATH='' cd -- "$(dirname -- "$0")/.." && pwd)
tmp_root=$(mktemp -d)
trap 'rm -rf "$tmp_root"' EXIT
cd "$tmp_root"

git init --quiet --initial-branch=main
git config user.name 'Release tests'
git config user.email 'release-tests@example.invalid'
git config commit.gpgsign false
git config tag.gpgsign false
GIT_EDITOR=true git commit --quiet --allow-empty -m 'test: initial commit'
main_sha=$(git rev-parse HEAD)
git update-ref refs/remotes/origin/main "$main_sha"
GIT_EDITOR=true git commit --quiet --allow-empty -m 'test: release candidate'
release_sha=$(git rev-parse HEAD)
git update-ref refs/remotes/origin/release/0.20.0 "$release_sha"
GIT_EDITOR=true git commit --quiet --allow-empty -m 'test: unrelated development'
other_sha=$(git rev-parse HEAD)

fail() {
  printf 'FAIL: %s\n' "$1" >&2
  exit 1
}

assert_valid() {
  local actual
  actual=$(bash "$repo_root/scripts/validate-release-tag.sh" "$1")
  [[ "$actual" == "$2" ]] || fail "unexpected commit for $1"
}

assert_invalid() {
  if bash "$repo_root/scripts/validate-release-tag.sh" "$1" > actual.txt 2> error.txt; then
    fail "expected $1 to fail"
  fi
  [[ ! -s actual.txt ]] || fail 'failed validation wrote a commit'
  grep -Fq "$2" error.txt || fail "expected error: $2"
}

git tag v0.19.0 "$main_sha"
assert_valid v0.19.0 "$main_sha"
GIT_EDITOR=true git tag -a v0.20.0-rc.1 "$release_sha" -m 'release v0.20.0-rc.1'
assert_valid v0.20.0-rc.1 "$release_sha"
git tag v0.20.0-beta.2 "$release_sha"
assert_valid v0.20.0-beta.2 "$release_sha"

# The candidate may be an ancestor of the current release branch tip.
git update-ref refs/remotes/origin/release/0.20.0 "$other_sha"
assert_valid v0.20.0-rc.1 "$release_sha"
git update-ref refs/remotes/origin/release/0.20.0 "$release_sha"

git tag v0.20.0 "$release_sha"
assert_invalid v0.20.0 'must reference a commit on main'
git tag v0.21.0-rc.1 "$release_sha"
assert_invalid v0.21.0-rc.1 'main or release/0.21.0'
git tag v0.20.0-rc.2 "$other_sha"
assert_invalid v0.20.0-rc.2 'main or release/0.20.0'

for tag in v0.20 v00.20.0 v0.20.0-rc.01 v0.20.0-01 v0.20.0- 'v0.20.0+build.1' '--help'; do
  assert_invalid "$tag" 'must match vX.Y.Z'
done
assert_invalid v9.0.0 'must already exist'

# A same-named branch is not a release tag.
git branch v9.0.0
assert_invalid v9.0.0 'must already exist'

# Tags pointing to non-commit objects must not pass validation.
blob_sha=$(printf 'not a commit' | git hash-object -w --stdin)
git tag v0.22.0 "$blob_sha"
assert_invalid v0.22.0 'must already exist and point to a commit'

# Promotion preserves the candidate commit; deleting the release branch is safe.
git update-ref refs/remotes/origin/main "$release_sha"
git update-ref -d refs/remotes/origin/release/0.20.0
assert_valid v0.20.0 "$release_sha"
assert_valid v0.20.0-rc.1 "$release_sha"

if bash "$repo_root/scripts/validate-release-tag.sh" > actual.txt 2> error.txt; then
  fail 'expected missing tag argument to fail'
fi
printf 'Release tag validation tests passed.\n'
