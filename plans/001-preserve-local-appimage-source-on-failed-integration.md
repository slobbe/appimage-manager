# Plan 001: Preserve local AppImage sources when integration fails

> **Executor instructions**: Follow this plan step by step. Run every verification command and confirm the expected result before moving to the next step. If anything in the "STOP conditions" section occurs, stop and report — do not improvise. When done, update the status row for this plan in `plans/README.md`.
>
> **Drift check (run first)**: `git diff --stat 57f5ab0..HEAD -- internal/app/service.go internal/app/service_test.go internal/infra/appimage/installer.go internal/infra/appimage/installer_test.go`
> If any in-scope file changed since this plan was written, compare the "Current state" excerpts against the live code before proceeding; on a mismatch, treat it as a STOP condition.

## Status

- **Priority**: P1
- **Effort**: M
- **Risk**: MED
- **Depends on**: none
- **Category**: bug
- **Planned at**: commit `57f5ab0`, 2026-06-13

## Why this matters

`aim add ./Example.AppImage` currently treats the user-provided local file as a movable install artifact. If later integration steps fail after the AppImage is installed, rollback deletes the installed copy but does not restore the original source path. That can lose a user's downloaded AppImage even though the command failed. The fix should make failed local integration non-destructive for the original user file while preserving update/GitHub staging behavior.

## Current state

Relevant files:

- `internal/app/service.go` — application workflow for local add and GitHub/update integration.
- `internal/app/service_test.go` — service-level integration tests with fakes for installer/remover failures.
- `internal/infra/appimage/installer.go` — filesystem adapter that installs AppImage files.
- `internal/infra/appimage/installer_test.go` — tests currently asserting source-removal semantics.

Current app workflow excerpt:

```go
// internal/app/service.go:245-251
installedAppImagePath, err := s.appImageInstaller.Install(ctx, req.Path, provisionalApp.ID)
if err != nil {
	return AddResult{}, err
}
rollback.add(func(ctx context.Context) error {
	return s.appImageRemover.Remove(ctx, installedAppImagePath)
})
```

Current installer excerpt:

```go
// internal/infra/appimage/installer.go:27-29
// Install moves sourcePath into the AppImage library as <appID>.AppImage and
// ensures the installed file is owner-executable.
func (i Installer) Install(ctx context.Context, sourcePath string, appID string) (string, error) {
```

```go
// internal/infra/appimage/installer.go:63-71
if err := os.Rename(sourcePath, destination); err == nil {
	return nil
}

if err := copyFile(ctx, sourcePath, destination); err != nil {
	return err
}
if err := os.Remove(sourcePath); err != nil {
	return fmt.Errorf("remove source after copy: %w", err)
}
```

Current test encoding destructive source behavior:

```go
// internal/infra/appimage/installer_test.go:35-37
if _, err := os.Stat(source); !errors.Is(err, os.ErrNotExist) {
	t.Fatalf("source stat error = %v, want not exist", err)
}
```

Repo conventions to follow:

- Keep domain/app/infra dependency direction from `AGENTS.md`: app defines ports; infra implements adapters; app must not import infra.
- Error handling is direct Go `error` returns with contextual wrapping in infra. Match existing style in `internal/infra/appimage/installer.go` and `internal/infra/download/downloader.go`.
- Tests are table/simple `testing` package tests with `t.TempDir`, fake ports, and `t.Fatalf`; no external assertion library.

## Commands you will need

| Purpose | Command | Expected on success |
|---|---|---|
| Focused app tests | `go test ./internal/app` | exit 0, all tests pass |
| Focused installer tests | `go test ./internal/infra/appimage` | exit 0, all tests pass |
| Full tests | `go test ./...` | exit 0, all packages pass |
| Vet | `go vet ./...` | exit 0, no diagnostics |
| Architecture | `make test-architecture` | exit 0, no grep output |
| Full local gate | `make verify` | exit 0 |

## Scope

**In scope**:

- `internal/app/service.go`
- `internal/app/service_test.go`
- `internal/infra/appimage/installer.go`
- `internal/infra/appimage/installer_test.go`
- `plans/README.md` status update only

**Out of scope**:

- Changing desktop/icon/storage behavior except as needed for rollback tests.
- Changing GitHub download behavior; that is covered by `plans/002-bound-and-verify-github-downloads.md`.
- Reworking update rollback beyond the source-preservation problem.
- Introducing new dependencies.

## Git workflow

- Do not create commits unless the operator explicitly asks.
- If committing later, use Conventional Commit style, e.g. `fix(add): preserve local appimage source on failure`.

## Steps

### Step 1: Add a failing characterization test for local add rollback

In `internal/app/service_test.go`, add a test near `TestServiceAddRollsBackAppImageWhenIconInstallFails` for local `AddRequest{Path: "/downloads/example.AppImage"}` where `deps.iconInstaller.err` or `deps.apps.err` fails after AppImage install.

The current fakes do not simulate real file movement, so assert the app-layer observable behavior available today:

- `deps.appImageInstaller.sourcePath` is still the local source path.
- rollback currently calls `deps.appImageRemover.Remove("/library/example-app.AppImage")`.

Then decide the production shape in Step 2 and adjust/add tests to prove the original local file is preserved at the infra level.

**Verify**: `go test ./internal/app` → initially may fail if you assert desired new behavior before implementing; after Step 2 it must pass.

### Step 2: Change installer semantics from move to copy

In `internal/infra/appimage/installer.go`, make `Installer.Install` copy the source file into the library instead of moving/removing it. Recommended minimal change:

- Update the comment from "moves" to "copies".
- Replace `moveFile(ctx, sourcePath, destination)` with a helper that copies to a temporary destination and renames into place.
- Remove the `os.Remove(sourcePath)` behavior from the install path.
- Keep owner-executable behavior (`ensureOwnerExecutable(destination)`) unchanged.
- Keep atomic-ish destination replacement through `destination + ".tmp"` and final `os.Rename`.

This intentionally preserves downloaded workspace files too; those are cleaned by `workspace.Cleanup()` later, so copying them does not leave permanent duplicates.

**Verify**: `go test ./internal/infra/appimage` → expect `TestInstallerInstallsAppImage` to fail until updated in Step 3.

### Step 3: Update installer tests for non-destructive source behavior

In `internal/infra/appimage/installer_test.go`:

- Rename or update `TestInstallerInstallsAppImage` so it asserts the source still exists after install and still contains `"appimage"`.
- Keep the destination content and owner-executable assertions.
- In `TestInstallerOverwritesExistingAppImage`, add an assertion that the source still exists after overwrite.
- Add a regression test that canceled context before install does not create/remove anything (if not already adequately covered by `TestInstallerRespectsCanceledContext`).

**Verify**: `go test ./internal/infra/appimage` → all tests pass.

### Step 4: Ensure app rollback still removes only installed artifacts

Run the app tests. If any test still expects source-removal behavior through fakes, update it to the new invariant: rollback removes installed destinations, not user source files.

Do not add app-layer file-copy logic. The app layer must remain filesystem-agnostic and continue to use the `AppImageInstaller` port.

**Verify**: `go test ./internal/app` → all tests pass.

### Step 5: Run full verification

Run the broader gates.

**Verify**:

- `go test ./...` → all packages pass.
- `go vet ./...` → exit 0.
- `make test-architecture` → exit 0.
- `make verify` → exit 0.

## Test plan

- Update `internal/infra/appimage/installer_test.go` to assert install copies and preserves source.
- Preserve existing overwrite and executable-mode coverage.
- Keep service rollback tests in `internal/app/service_test.go` focused on installed artifact removal.
- Use existing tests as structural patterns: `TestInstallerInstallsAppImage`, `TestServiceAddRollsBackInstalledArtifactsOnRepositoryFailure`, and `TestServiceAddRollsBackAppImageWhenIconInstallFails`.

## Done criteria

- [ ] `internal/infra/appimage/installer.go` no longer removes the source file on successful install.
- [ ] Installer comment accurately says copy/install, not move.
- [ ] `go test ./internal/infra/appimage` passes and includes source-preservation assertions.
- [ ] `go test ./internal/app` passes.
- [ ] `go test ./...`, `go vet ./...`, `make test-architecture`, and `make verify` pass.
- [ ] No files outside the in-scope list are modified except `plans/README.md` status.
- [ ] `plans/README.md` status row updated.

## STOP conditions

Stop and report if:

- The code at the current-state excerpts does not match live code after the drift check.
- Preserving the source appears to require changing the app port interface in `internal/app`.
- You discover `workspace.Cleanup()` does not remove GitHub/update downloaded files, making copy semantics leak cache/workspace artifacts.
- Any verification command fails twice after focused fixes.

## Maintenance notes

Reviewers should scrutinize overwrite behavior and rollback semantics. The key invariant after this plan is: installing into the managed library must not destroy the caller's source path; cleanup of temporary download/workspace files remains the workspace provider's responsibility.