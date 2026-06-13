# Plan 008: Serialize repository writes to prevent concurrent state loss

> **Executor instructions**: Follow this plan step by step. Run every verification command and confirm the expected result before moving to the next step. If anything in the "STOP conditions" section occurs, stop and report — do not improvise. When done, update the status row for this plan in `plans/README.md`.
>
> **Drift check (run first)**: `git diff --stat 0cee363..HEAD -- internal/infra/storage/repository.go internal/infra/storage/repository_test.go`
> If any in-scope file changed since this plan was written, compare the "Current state" excerpts against live code before proceeding; on mismatch, treat it as a STOP condition.

## Status

- **Priority**: P1
- **Effort**: M
- **Risk**: MED
- **Depends on**: none
- **Category**: bug
- **Planned at**: commit `0cee363`, 2026-06-13

## Why this matters

The storage repository performs load-modify-save with no cross-process coordination. Two concurrent CLI processes can both read the same `apps.json`, make different changes, and the later writer can overwrite the earlier writer's change. The shared temporary path `apps.json.tmp` also allows concurrent writers to interfere with each other's pending writes. `aim` manages user state, so state loss during normal multi-terminal usage is a correctness bug.

## Current state

Relevant files:

- `internal/infra/storage/repository.go` — JSON repository implementation for managed apps.
- `internal/infra/storage/repository_test.go` — focused repository tests.

Current write paths:

```go
// internal/infra/storage/repository.go:142-161
func (r Repository) Save(ctx context.Context, domainApp domain.App) error {
    // ...
    db, err := r.load(ctx)
    // modify db.Apps
    sortAppRecords(db.Apps)
    return r.save(ctx, db)
}
```

```go
// internal/infra/storage/repository.go:233-241
func (r Repository) Delete(ctx context.Context, id string) error {
    db, err := r.load(ctx)
    // remove record
    return r.save(ctx, db)
}
```

```go
// internal/infra/storage/repository.go:295-303
temporaryPath := r.Path + ".tmp"
if err := os.WriteFile(temporaryPath, bytes, 0o644); err != nil { ... }
if err := os.Rename(temporaryPath, r.Path); err != nil { ... }
```

Repo conventions to follow:

- Infra adapters may use `golang.org/x/sys` (already required in `go.mod`) for Linux-specific system calls.
- Return contextual errors with `fmt.Errorf`.
- Tests use `t.TempDir`, plain `testing`, and no assertion libraries.
- Keep repository API changes minimal; app layer currently depends on `Save`, `Find`, `List`, and `Delete`.

## Commands you will need

| Purpose | Command | Expected on success |
|---|---|---|
| Focused storage tests | `go test ./internal/infra/storage` | exit 0 |
| Race check for storage | `go test -race ./internal/infra/storage` | exit 0 |
| Full tests | `go test ./...` | exit 0 |
| Vet | `go vet ./...` | exit 0 |
| Architecture | `make test-architecture` | exit 0 |
| Full gate | `make verify` | exit 0 |

## Scope

**In scope**:

- `internal/infra/storage/repository.go`
- `internal/infra/storage/repository_test.go`
- `go.mod`/`go.sum` only if required for an already-approved locking package; prefer existing `golang.org/x/sys/unix`
- `plans/README.md` status update only

**Out of scope**:

- Changing the persisted JSON schema.
- Changing app-layer repository port names unless absolutely necessary.
- Implementing bulk-save/transactions for performance; that was not selected as this plan's goal.
- Locking unrelated filesystem adapters.

## Git workflow

- Do not create commits unless the operator explicitly asks.
- If committing later, use a message like `fix(storage): serialize app database writes`.

## Steps

### Step 1: Add a concurrency regression test

In `internal/infra/storage/repository_test.go`, add a test that demonstrates two concurrent `Save` calls preserve both records. A stable approach:

1. Create a repository at `t.TempDir()/apps.json`.
2. Start two goroutines that call `Save` with different app IDs.
3. Repeat the race in a loop enough times to catch the old lost-update behavior, or add a test-only hook if the repository already has suitable seams.
4. After both saves complete, call `List` and assert both IDs exist.

If this cannot be made deterministic without production hooks, add a more direct test around the new lock helper in Step 2 and document why a deterministic lost-update test is impractical.

**Verify**: `go test -race ./internal/infra/storage` → should fail or be flaky before the fix if using direct concurrency; after the fix it must pass consistently.

### Step 2: Add a repository lock helper

In `repository.go`, add a private helper that acquires an advisory file lock next to the database file, for example `apps.json.lock`. Recommended Linux implementation using the existing `golang.org/x/sys/unix` dependency:

- Ensure `filepath.Dir(r.Path)` exists before opening the lock file.
- Open/create the lock file with `os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o644)`.
- Use `unix.Flock(int(file.Fd()), unix.LOCK_EX)` to acquire and `LOCK_UN` in a cleanup function.
- Ensure unlock/close happens on every return path.
- Check `ctx.Err()` before waiting and after acquiring. If blocking lock acquisition with context cancellation is a concern, STOP and report rather than inventing a complex polling protocol.

Keep this helper private to the storage package.

**Verify**: `go test ./internal/infra/storage` → may still pass; no behavior switched yet.

### Step 3: Lock around read-modify-write mutations

Update `Save` and `Delete` so the lock covers the entire load-modify-save sequence. It is not sufficient to lock only `save`, because the stale read is part of the race.

Target shape:

```go
unlock, err := r.lock(ctx)
if err != nil { return err }
defer unlock()

db, err := r.load(ctx)
// mutate
return r.save(ctx, db)
```

Avoid acquiring the same lock in `load`, `Find`, or `List`; reads can remain lock-free unless tests reveal partial-read issues. If you decide reads must lock too, keep the change focused and document the reason in the code or tests.

**Verify**: `go test -race ./internal/infra/storage` → exit 0.

### Step 4: Use unique temporary files for saves

Replace the shared `r.Path + ".tmp"` temp path with a unique temp file in the same directory, using `os.CreateTemp(filepath.Dir(r.Path), filepath.Base(r.Path)+".*.tmp")`. Write the JSON, close it, then `os.Rename(tmp.Name(), r.Path)`. On any error, remove that specific temp file.

This reduces interference even if a future write path forgets to hold the lock.

**Verify**: `go test ./internal/infra/storage` → exit 0.

### Step 5: Run full verification

**Verify**:

- `go test -race ./internal/infra/storage` → exit 0.
- `go test ./...` → all packages pass.
- `go vet ./...` → exit 0.
- `make test-architecture` → exit 0.
- `make verify` → exit 0.

## Test plan

- Add concurrent-save coverage in `internal/infra/storage/repository_test.go` ensuring two records survive concurrent writes.
- Add or update tests proving temp files are cleaned up on write/rename errors if existing test seams make that practical.
- Use existing repository tests as the style pattern; do not add dependencies.

## Done criteria

- [ ] `Save` and `Delete` hold an advisory lock across load-modify-save.
- [ ] `save` uses unique temp files, not a fixed `apps.json.tmp` path.
- [ ] Concurrent storage test exists and passes under `go test -race ./internal/infra/storage`.
- [ ] `go test ./...`, `go vet ./...`, `make test-architecture`, and `make verify` exit 0.
- [ ] Persisted JSON schema and public app-layer contracts are unchanged.
- [ ] `plans/README.md` status row updated.

## STOP conditions

Stop and report back if:

- The target platform needs non-Linux lock semantics beyond this repo's Linux AppImage scope.
- You cannot implement locking without adding a new dependency or changing app-layer ports.
- The concurrency test remains flaky after the locking fix.
- Any in-scope file has drifted substantially from the excerpts.

## Maintenance notes

Future repository mutation methods must use the same lock helper around the full read-modify-write sequence. Reviewers should look for accidental nested locks and verify temp files are created in the same directory as the database so final rename remains atomic on the filesystem.
