# Releasing `aim`

This document describes the release flow.

## Branches and tags

- `main` is the stable source branch, not the integration branch for unreleased work.
- Develop the next version on a branch named `release/X.Y.Z`, such as `release/0.20.0`. Contributors branch from it and target it with pull requests.
- Use immutable, annotated, SemVer tags with a lowercase `v` prefix: `v0.20.0-rc.1`, `v0.20.0-rc.2`, then `v0.20.0`.
- Cut release candidates from the matching release branch. A prerelease tag's commit must be reachable from `release/X.Y.Z` for its base version, or from `main`.
- A stable tag's commit must be reachable from `main` before publication. Merely having equivalent changes on `main` is not sufficient.
- After the first RC, do not rebase, squash, or otherwise rewrite the release branch's history. Add fixes as new commits and tag a new RC.
- Never move, replace, or delete a published release tag. A code fix requires a new version, not a retag.
