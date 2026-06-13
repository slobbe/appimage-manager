# Plan 007: Avoid chmodding the user's original AppImage during integration

> **Executor instructions**: Follow this plan step by step. Run every verification command and confirm the expected result before moving to the next step. If anything in the "STOP conditions" section occurs, stop and report — do not improvise. When done, update the status row for this plan in `plans/README.md`.
>
> **Drift check (run first)**: `git diff --stat 0cee363..HEAD -- internal/app/service.go internal/app/service_test.go internal/app/integration_ports.go internal/infra/appimage/extractor.go internal/infra/appimage/extractor_test.go`
> If any in-scope file changed since this plan was written, compare the "Current state" excerpts against live code before proceeding; on mismatch, treat it as a STOP condition.

## Status

- **Priority**: P1
- **Effort**: M
- **Risk**: MED
- **Depends on**: none
- **Category**: bug
- **Planned at**: commit `0cee363`, 2026-06-13

## Why this matters

Plan 001 changed AppImage installation to copy rather than move, but extraction still changes permissions on the user-provided source file. `aim add /path/to/local.AppImage` can therefore mutate the original file with `chmod u+x` before the file is managed by `aim`. This is surprising, can fail on read-only locations, and leaves source metadata changed even if later integration fails. The fix should stage local AppImages in a workspace before extraction and only chmod the staged copy.

## Current state

Relevant files:

- `internal/app/service.go` — coordinates local/GitHub/update integration and owns workspaces.
- `internal/infra/appimage/extractor.go` — executes AppImages with `--appimage-updateinformation` and `--appimage-extract`.
- `internal/infra/appimage/extractor_test.go` — focused extractor tests; use it for chmod behavior if practical.
- `internal/app/integration_ports.go` — app-layer port definitions; adjust only if a helper/port contract needs clarification.

Current extractor behavior:

```go
// internal/infra/appimage/extractor.go:27-33
// Extract extracts appImagePath into destDir and returns the extracted root.
//
// AppImage extraction creates a squashfs-root directory in the process working
// directory, so the command is run with destDir as its working directory. The
// source AppImage is made owner-executable before extraction because AppImages
// must be executable to run their extraction mode.
func (Extractor) Extract(ctx context.Context, appImagePath string, destDir string) (app.AppImageExtraction, error) {
```

```go
// internal/infra/appimage/extractor.go:53-57
if err := ensureOwnerExecutable(absoluteAppImagePath); err != nil {
    return app.AppImageExtraction{}, err
}

updateInfo, err := appImageUpdateInfo(ctx, absoluteAppImagePath)
```

```go
// internal/infra/appimage/extractor.go:102-118
func ensureOwnerExecutable(path string) error {
    info, err := os.Stat(path)
    // ...
    if mode&0o100 != 0 { return nil }
    if err := os.Chmod(path, mode|0o100); err != nil {
        return fmt.Errorf("make appimage executable %q: %w", path, err)
    }
    return nil
}
```

Current app workflow passes the user path directly to the extractor:

```go
// internal/app/service.go:212-219
workspace, err := s.workspaces.Create(ctx)
// ...
extraction, err := s.appImages.Extract(ctx, req.Path, filepath.Join(workspace.Path, "extract"))
```

Repo conventions to follow:

- App layer defines workflows/ports; infra implements filesystem details.
- Do not import `internal/infra` from `internal/app`.
- Use `t.TempDir`, simple fakes, and `testing` package only.
- Keep local, GitHub, and update integration behavior consistent unless explicitly separated.

## Commands you will need

| Purpose | Command | Expected on success |
|---|---|---|
| Focused app tests | `go test ./internal/app` | exit 0 |
| Focused extractor tests | `go test ./internal/infra/appimage` | exit 0 |
| Full tests | `go test ./...` | exit 0 |
| Vet | `go vet ./...` | exit 0 |
| Architecture | `make test-architecture` | exit 0 |
| Full gate | `make verify` | exit 0 |

## Scope

**In scope**:

- `internal/app/service.go`
- `internal/app/service_test.go`
- `internal/app/integration_ports.go` only if comments/types need a narrow update
- `internal/infra/appimage/extractor.go`
- `internal/infra/appimage/extractor_test.go`
- `plans/README.md` status update only

**Out of scope**:

- Changing AppImage installer copy semantics from Plan 001.
- Changing download size enforcement from Plan 002.
- Reworking desktop/icon discovery.
- Adding external dependencies.

## Git workflow

- Do not create commits unless the operator explicitly asks.
- If committing later, use a message like `fix(add): avoid chmodding source appimage`.

## Steps

### Step 1: Add an app-level regression test for source staging intent

In `internal/app/service_test.go`, add or adjust fake extractor assertions so local add does not pass the original user path directly to `AppImageExtractor.Extract`. The fake should record the `appImagePath` argument. For a local request like `AddRequest{Path: "/downloads/example.AppImage"}`, desired behavior is that extraction receives a path inside the workspace/staging area, not `/downloads/example.AppImage`.

If existing fakes do not model workspace paths clearly, extend the test setup minimally. Do not assert exact random temp paths; assert stable properties such as `extractor.path != original` and `strings.Contains(extractor.path, workspace.Path)`.

**Verify**: `go test ./internal/app` → the new test should fail before implementation.

### Step 2: Stage local AppImages before extraction

In `internal/app/service.go`, change `integrateLocal` so it copies `req.Path` into the existing workspace before calling `s.appImages.Extract`. Recommended shape:

1. Create workspace as today.
2. Copy the AppImage from `req.Path` to a staged path under `workspace.Path`, preserving content and using a safe filename such as `filepath.Base(req.Path)`.
3. Pass the staged path to `s.appImages.Extract(ctx, stagedPath, filepath.Join(workspace.Path, "extract"))`.
4. Continue installing from the original source or the correct source according to current installer semantics. If preserving original local source requires installing from original path, keep that. If GitHub/update downloads already live in a workspace, staging can still copy within workspace; acceptable but avoid unnecessary mutation of user-owned paths.

Keep the copy helper in the app layer only if it uses generic filesystem operations and does not introduce infra imports. Alternatively, define an app-layer file-copy/staging port and implement it in infra only if necessary. Prefer the smallest change that preserves architecture.

**Verify**: `go test ./internal/app` → focused app tests pass.

### Step 3: Keep extractor chmod behavior but clarify its contract

`internal/infra/appimage/extractor.go` may still call `ensureOwnerExecutable` because the staged file must be executable. Update the comment to clarify that callers should pass a staged/managed path when they do not want the original file mutated.

Do not remove `ensureOwnerExecutable` entirely unless tests prove extraction can still work for non-executable staged AppImages.

**Verify**: `go test ./internal/infra/appimage` → exit 0.

### Step 4: Add or update extractor tests for chmod locality if practical

If `internal/infra/appimage/extractor_test.go` already has command-execution fakes or helpers, add a focused unit test for `ensureOwnerExecutable` behavior on a temp file. The important invariant for this plan is enforced at the app workflow level, so do not over-engineer extractor tests.

**Verify**: `go test ./internal/infra/appimage` → exit 0.

### Step 5: Run full verification

**Verify**:

- `go test ./...` → all packages pass.
- `go vet ./...` → exit 0.
- `make test-architecture` → exit 0.
- `make verify` → exit 0.

## Test plan

- `internal/app/service_test.go`: regression test that local integration passes a staged path to the extractor, not the original user path.
- Existing app tests for local add, GitHub add, and update must continue passing.
- Optional `internal/infra/appimage/extractor_test.go`: verify `ensureOwnerExecutable` on a temp/staged file if helper coverage exists.

## Done criteria

- [ ] Local `AddRequest{Path: original}` no longer passes `original` directly to `AppImageExtractor.Extract`.
- [ ] Original local file permissions are not changed by app-layer tests/fakes that model extraction staging.
- [ ] `go test ./internal/app` exits 0.
- [ ] `go test ./internal/infra/appimage` exits 0.
- [ ] `go test ./...`, `go vet ./...`, `make test-architecture`, and `make verify` exit 0.
- [ ] No layer dependency rules are violated.
- [ ] `plans/README.md` status row updated.

## STOP conditions

Stop and report back if:

- The fix appears to require `internal/app` importing `internal/infra`.
- Existing installer semantics make it impossible to both preserve the original file and install the intended file without a broader port redesign.
- The code at `internal/app/service.go:212-219` or `internal/infra/appimage/extractor.go:53-57` no longer resembles the excerpts.
- Any full verification command fails twice after focused fixes.

## Maintenance notes

Future extraction features should treat user-provided paths as immutable unless the command explicitly documents mutation. Reviewers should check that workspace cleanup still removes staged copies and that GitHub/update flows do not accidentally install from a cleaned-up path.
