# Plan 020: Decide the roadmap for non-GitHub update sources

> **Executor instructions**: Follow this design plan step by step. Do not implement zsync/local-file updates in this plan. Produce a clear decision and align wording/tests with that decision. If a STOP condition occurs, stop and report. Update `plans/README.md` when done.
>
> **Drift check (run first)**: `git diff --stat 0cee363..HEAD -- internal/domain/app.go internal/app/service.go internal/app/service_test.go internal/cli/command/info/command.go README.md docs`

## Status

- **Priority**: P3
- **Effort**: S for decision/docs, L for future implementation
- **Risk**: LOW for decision/docs
- **Depends on**: none
- **Category**: direction
- **Planned at**: commit `0cee363`, 2026-06-13

## Why this matters

The domain/storage/info layers preserve `local_file`, `zsync`, and `unsupported` update source metadata, but update planning only acts on GitHub sources. Users with AppImages that expose embedded zsync update info may reasonably expect updates to work. The project should decide whether non-GitHub sources are roadmap, legacy metadata, or explicitly unsupported for now.

## Current state

Evidence:

```go
// internal/domain/app.go:43-49
const (
    UpdateSourceKindLocalFile UpdateSourceKind = "local_file"
    UpdateSourceKindGitHub    UpdateSourceKind = "github"
    UpdateSourceKindZsync     UpdateSourceKind = "zsync"
    UpdateSourceKindUnsupported UpdateSourceKind = "unsupported"
)
```

```go
// internal/app/service.go:399-400
if installedApp.UpdateSource.Kind != domain.UpdateSourceKindGitHub || strings.TrimSpace(installedApp.UpdateSource.Repo) == "" {
    continue
}
```

A service test at plan time was named `TestServiceUpdateSkipsEmbeddedZsyncSourceForNow`, encoding that zsync skipping is intentional for now.

## Commands you will need

| Purpose | Command | Expected on success |
|---|---|---|
| App tests | `go test ./internal/app` | exit 0 |
| CLI info tests | `go test ./internal/cli/command/info` | exit 0 if touched |
| Full tests | `go test ./...` | exit 0 |
| Full gate | `make verify` | exit 0 |

## Scope

**In scope**:

- A short decision note under `docs/adr/` if that directory exists, otherwise under `plans/020-non-github-update-source-decision.md`
- README wording about supported update sources
- `internal/app/service_test.go` test names/messages only if clarifying skipped behavior
- `internal/cli/command/info/command.go` wording only if it implies support incorrectly
- `plans/README.md` status update only

**Out of scope**:

- Implementing zsync update downloads.
- Implementing local-file update semantics.
- Changing persisted update source schema.

## Steps

### Step 1: Write the decision

Write a decision note that chooses one:

1. **Metadata-only for now**: preserve/display non-GitHub sources, but update skips them.
2. **Roadmap later**: preserve/display now and create follow-up implementation plans for zsync/local-file.
3. **Remove/deprecate**: stop surfacing unsupported sources beyond raw metadata.

Given current code and risk, prefer option 1 or 2. Document security implications of zsync/local-file updates.

**Verify**: decision note exists and states status, consequences, and follow-up.

### Step 2: Align user-facing wording

Update README or CLI help/info wording so users understand GitHub is the only applied update source today. If `info` displays `zsync`, include wording such as "preserved; update not applied by aim yet" only if the output format allows this without breaking JSON.

**Verify**: `go test ./internal/cli/command/info` if CLI code changed.

### Step 3: Align tests/names

If existing tests say "ForNow", either keep that if the decision is roadmap-later, or rename to the chosen permanent behavior. Add a test if needed to assert non-GitHub sources are skipped and do not error.

**Verify**: `go test ./internal/app` → exit 0.

### Step 4: Run full verification

**Verify**:

- `go test ./...` → exit 0.
- `make verify` → exit 0.

## Done criteria

- [ ] A decision note exists and clearly states current/projected support for `local_file`, `zsync`, and `unsupported` sources.
- [ ] README/CLI wording no longer implies non-GitHub sources are applied if they are not.
- [ ] Tests match the chosen decision.
- [ ] No implementation of new update transports was attempted.
- [ ] `go test ./...` and `make verify` pass.
- [ ] `plans/README.md` status row updated.

## STOP conditions

Stop and report if:

- The operator wants real zsync implementation in this plan.
- Wording changes would break existing JSON schemas.
- There is already an ADR/docs decision contradicting the recommended choice.

## Maintenance notes

If the project later implements zsync/local-file updates, start with a separate design covering transport security, integrity verification, rollback, and version comparison semantics.
