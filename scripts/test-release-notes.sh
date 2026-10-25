#!/usr/bin/env bash
set -euo pipefail

repo_root=$(CDPATH='' cd -- "$(dirname -- "$0")/.." && pwd)
tmp_root=$(mktemp -d)
trap 'rm -rf "$tmp_root"' EXIT
cd "$tmp_root"

fail() {
  printf 'FAIL: %s\n' "$1" >&2
  exit 1
}

assert_notes() {
  bash "$repo_root/scripts/release-notes.sh" "$1" > actual.md
  diff -u expected.md actual.md || fail "unexpected notes for $1"
}

assert_failure() {
  if bash "$repo_root/scripts/release-notes.sh" v0.20.0 > actual.md 2> error.txt; then
    fail 'expected extraction to fail'
  fi
  [[ ! -s actual.md ]] || fail 'failed extraction wrote release notes'
  grep -Fq "$1" error.txt || fail "expected error: $1"
}

cat > CHANGELOG.md <<'EOF'
# Changelog

## v0.21.0 (unreleased)

### New

- Added something newer.

## v0.20.0 - 2026-10-25

### Changed

- Made updates concurrent.

### Fixed

- Fixed an update issue.

## v0.19.0

### New

- Added something older.
EOF
cat > expected.md <<'EOF'
### Changed

- Made updates concurrent.

### Fixed

- Fixed an update issue.
EOF
assert_notes v0.20.0

cat > CHANGELOG.md <<'EOF'
# Changelog

## v0.20.0 (unreleased)

### Changed

- Made updates concurrent.
EOF
cat > expected.md <<'EOF'
### Changed

- Made updates concurrent.
EOF
for tag in v0.20.0 v0.20.0-preview.1 v0.20.0-dev v0.20.0-rc.2; do
  assert_notes "$tag"
done

for heading in v0.20.01 v0.20.0-preview.1; do
  printf '## %s\n\n- Other release.\n' "$heading" > CHANGELOG.md
  assert_failure 'has no entry for v0.20.0'
done

printf '# Changelog\n' > CHANGELOG.md
assert_failure 'has no entry for v0.20.0'

for body in '' $'\n' $'\n### Changed\n'; do
  printf '## v0.20.0\n%s\n## v0.19.0\n\n- Older change.\n' "$body" > CHANGELOG.md
  assert_failure 'has no changelog bullets'
done

rm CHANGELOG.md
if bash "$repo_root/scripts/release-notes.sh" v0.20.0 > actual.md 2> error.txt; then
  fail 'expected missing changelog file to fail'
fi
if bash "$repo_root/scripts/release-notes.sh" > actual.md 2> error.txt; then
  fail 'expected missing tag argument to fail'
fi

printf 'Release notes tests passed.\n'
