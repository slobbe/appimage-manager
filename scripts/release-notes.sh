#!/usr/bin/env bash
set -euo pipefail

if [[ $# -ne 1 ]]; then
  echo 'usage: bash scripts/release-notes.sh <release-tag>' >&2
  exit 1
fi

# Prereleases share the stable version's changelog entry.
version=${1%%-*}

awk -v version="$version" '
  /^##[ \t]+/ {
    if (found) exit
    found = ($2 == version)
    next
  }
  found {
    notes = notes $0 "\n"
    if ($0 ~ /^-[ \t]+[^ \t]/) has_bullet = 1
  }
  END {
    if (!found) {
      print "CHANGELOG.md has no entry for " version > "/dev/stderr"
      exit 1
    }
    if (!has_bullet) {
      print "CHANGELOG.md entry for " version " has no changelog bullets" > "/dev/stderr"
      exit 1
    }
    sub(/^[[:space:]]+/, "", notes)
    sub(/[[:space:]]+$/, "", notes)
    print notes
  }
' CHANGELOG.md
