# Plan 009: Make v1 migration transactional before mutating user artifacts

> **Executor instructions**: Follow this plan step by step. Run every verification command and confirm the expected result before moving to the next step. If anything in the "STOP conditions" section occurs, stop and report — do not improvise. When done, update the status row for this plan in `plans/README.md`.
>
> **Drift check (run first)**: `git diff --stat 0cee363..HEAD -- internal/infra/migration/v1.go internal/infra/migration/v1_test.go`
> If any in-scope file changed since this plan was written, compare the "Current state" excerpts against live code before proceeding; on mismatch, treat it as a STOP condition.

## Status

- **Priority**: P1
- **Effort**: M
- **Risk**: MED
- **Depends on**: none
- **Category**: migration
- **Planned at**: commit `0cee363`, 2026-06-13

## Why this matters

The v1 migration moves AppImages and rewrites desktop entries before it writes the new v2 database. If `writeDatabase` fails after those mutations, `aim` can leave the legacy database in place while files and desktop entries now point to new paths. Migration runs at startup in `cmd/aim/main.go`, so a failure can strand user-managed state before any command runs. The migration should either preflight and write safely or roll back every artifact mutation on failure.

## Current state

Relevant files:

- `internal/infra/migration/v1.go` — v1-to-v2 migration logic and filesystem mutations.
- `internal/infra/migration/v1_test.go` — migration tests; add rollback/failure coverage here.
- `cmd/aim/main.go` — calls migration at startup; do not modify unless a STOP condition is reached.

Current mutation order:

```go
// internal/infra/migration/v1.go:69-83
for _, plan := range plans {
    if err := moveIfNeeded(plan.oldAppImagePath, plan.newAppImagePath); err != nil {
        return false, err
    }
    if err := updateDesktopEntry(plan.desktopEntryPath, plan.newAppImagePath, plan.id); err != nil {
        return false, err
    }
}

if err := writeDatabase(opts.DestPath, v2); err != nil {
    return false, err
}
```

Current artifact mutations:

```go
// internal/infra/migration/v1.go:387-389
if err := os.Rename(src, dst); err != nil {
    return fmt.Errorf("move appimage %q to %q: %w", src, dst, err)
}
```

```go
// internal/infra/migration/v1.go:429-435
tmp := path + ".tmp"
if err := os.WriteFile(tmp, []byte(content), info.Mode().Perm()); err != nil { ... }
if err := os.Rename(tmp, path); err != nil { ... }
```

Repo conventions to follow:

- Migration code lives in infra and may perform filesystem I/O.
- Tests should use temp directories/files and plain `testing` helpers.
- Prefer explicit rollback/preflight code over broad abstractions.
- Do not change domain/app layer for this migration fix.

## Commands you will need

| Purpose | Command | Expected on success |
|---|---|---|
| Focused migration tests | `go test ./internal/infra/migration` | exit 0 |
| Full tests | `go test ./...` | exit 0 |
| Vet | `go vet ./...` | exit 0 |
| Architecture | `make test-architecture` | exit 0 |
| Full gate | `make verify` | exit 0 |

## Scope

**In scope**:

- `internal/infra/migration/v1.go`
- `internal/infra/migration/v1_test.go`
- `plans/README.md` status update only

**Out of scope**:

- Changing the v2 database schema.
- Changing normal storage repository behavior; see Plan 008.
- Rewriting old migration plan generation unless needed to preflight safely.
- Modifying `cmd/aim/main.go` startup wiring.

## Git workflow

- Do not create commits unless the operator explicitly asks.
- If committing later, use a message like `fix(migration): roll back v1 artifact changes on failure`.

## Steps

### Step 1: Add failure tests for post-mutation database-write errors

In `internal/infra/migration/v1_test.go`, add a test that forces `writeDatabase(opts.DestPath, v2)` to fail after at least one AppImage move and one desktop entry rewrite would have occurred.

Use temp directories. Possible ways to force failure:

- Set `DestPath` under a path that cannot be written, if reliable on Linux.
- Create a directory at the exact `DestPath` so writing a file there fails.
- If existing helpers make this hard, refactor `writeDatabase` behind a package-level variable only if necessary; reset it with `t.Cleanup`.

Assert after `MigrateV1` returns an error:

- Original AppImage path still exists.
- New AppImage path does not exist, or if it exists due to rollback failure, the test must fail.
- Desktop entry content matches the original content.
- Legacy source database still exists.

**Verify**: `go test ./internal/infra/migration` → new test should fail before implementation.

### Step 2: Add rollback tracking for artifact mutations

In `v1.go`, introduce local rollback tracking inside `MigrateV1` around the artifact mutation loop. For each successful `moveIfNeeded(src, dst)`, register a rollback that moves `dst` back to `src` if `src != dst`. For each successful desktop rewrite, capture the original bytes and file mode before writing so rollback can restore them.

Recommended implementation:

- Create a small private `migrationRollback` stack similar in spirit to `internal/app/rollback.go`, but keep it inside `internal/infra/migration` to avoid cross-layer imports.
- Execute rollbacks in reverse order on any error after a mutation.
- If rollback itself fails, return an error that includes both the original error and rollback failure context. Do not silently swallow rollback failures.

**Verify**: `go test ./internal/infra/migration` → the new test should pass or reveal missing desktop rollback handling.

### Step 3: Refactor desktop entry update to support restoration

If `updateDesktopEntry` currently only returns `error`, adjust it to report whether it changed the file and enough information to restore original bytes/mode. Keep the function private. One acceptable shape:

```go
type desktopEntryBackup struct {
    path string
    bytes []byte
    mode os.FileMode
    changed bool
}
```

Do not change desktop entry rewriting semantics beyond making it rollback-safe.

**Verify**: `go test ./internal/infra/migration` → exit 0.

### Step 4: Add preflight checks where cheap

Before mutating artifacts, add preflight checks for obvious failures:

- Destination database directory can be created.
- Existing destination path is not a directory, or if it is, fail before mutations.
- Planned destination AppImage paths do not already exist, matching current `moveIfNeeded` behavior.

Preflight is not a substitute for rollback, because filesystem state can still change between preflight and write.

**Verify**: `go test ./internal/infra/migration` → exit 0.

### Step 5: Run full verification

**Verify**:

- `go test ./internal/infra/migration` → all tests pass.
- `go test ./...` → all packages pass.
- `go vet ./...` → exit 0.
- `make test-architecture` → exit 0.
- `make verify` → exit 0.

## Test plan

- Add database-write-failure rollback coverage in `internal/infra/migration/v1_test.go`.
- Add a test for desktop-entry rewrite failure after an AppImage move if not already covered.
- Keep existing successful migration tests passing unchanged.
- Use existing migration tests as structural examples.

## Done criteria

- [ ] If database write fails after artifact mutations, moved AppImages are restored to original paths.
- [ ] Rewritten desktop entries are restored to original content/mode on later failure.
- [ ] Preflight catches obvious unwritable/invalid destination database paths before artifact mutation.
- [ ] `go test ./internal/infra/migration` exits 0 with new rollback tests.
- [ ] `go test ./...`, `go vet ./...`, `make test-architecture`, and `make verify` exit 0.
- [ ] No database schema changes were made.
- [ ] `plans/README.md` status row updated.

## STOP conditions

Stop and report back if:

- You cannot make database write failure deterministic in tests without invasive production hooks.
- Rollback can fail in common cases and there is no safe way to report/recover without a larger migration design.
- Fixing this requires changing `cmd/aim/main.go` startup behavior.
- In-scope code has drifted from the excerpts.

## Maintenance notes

Future migrations that move user artifacts should be designed as preflight + mutate + rollback from the start. Reviewers should scrutinize failure paths more than happy paths and should manually inspect that rollback order is reverse mutation order.
