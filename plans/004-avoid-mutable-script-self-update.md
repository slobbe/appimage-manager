# Plan 004: Avoid self-updating from a mutable branch script

> **Executor instructions**: Follow this plan step by step. Run every verification command and confirm the expected result before moving to the next step. If anything in the "STOP conditions" section occurs, stop and report — do not improvise. When done, update the status row for this plan in `plans/README.md`.
>
> **Drift check (run first)**: `git diff --stat 57f5ab0..HEAD -- internal/app/service.go internal/app/selfupdate_ports.go internal/infra/selfupdate/installer.go internal/infra/selfupdate/installer_test.go scripts/install.sh .goreleaser.yaml`
> If any in-scope file changed since this plan was written, compare the "Current state" excerpts against the live code before proceeding; on a mismatch, treat it as a STOP condition.

## Status

- **Priority**: P1
- **Effort**: M
- **Risk**: MED
- **Depends on**: `plans/003-add-selfupdate-installer-characterization-tests.md`
- **Category**: security
- **Planned at**: commit `57f5ab0`, 2026-06-13

## Why this matters

`aim selfupdate` asks the app layer for a GitHub release version, but the infra adapter executes the installer script from the mutable `main` branch. That means the code run during self-update is not tied to the release the user confirmed. The safer near-term fix is to fetch the installer script from the selected immutable tag (or a commit/tag-derived URL) rather than from `main`, while preserving the existing installer-script workflow.

## Current state

Relevant files:

- `internal/app/service.go` — resolves the release and calls `s.selfUpdater.Install(ctx, version)`.
- `internal/app/selfupdate_ports.go` — app-defined port for self-update adapter.
- `internal/infra/selfupdate/installer.go` — downloads and executes the install script.
- `internal/infra/selfupdate/installer_test.go` — should exist after Plan 003.
- `scripts/install.sh` — script currently used by both README curl install and self-update.
- `.goreleaser.yaml` — release asset naming, useful if choosing a binary/archive approach instead of tagged script.

Current app flow excerpt:

```go
// internal/app/service.go:741-779
release, err := s.selfUpdateRelease(ctx, req.Prerelease, activity)
// ... candidate and confirmation ...
task := activity.Start(ctx, Activity{Kind: ActivityKindWaiting, AppID: "selfupdate"})
if err := s.selfUpdater.Install(ctx, version); err != nil {
	task.Fail(err)
	return SelfUpdateResult{}, err
}
task.Done("Updated aim to " + candidate.NewVersion)
```

Current infra excerpt:

```go
// internal/infra/selfupdate/installer.go:15
const defaultInstallScriptURL = "https://raw.githubusercontent.com/slobbe/appimage-manager/main/scripts/install.sh"
```

```go
// internal/infra/selfupdate/installer.go:38-46
script, err := i.fetchInstallScript(ctx)
// ...
cmd := exec.CommandContext(ctx, "sh")
cmd.Stdin = script
cmd.Env = append(cmd.Environ(), "AIM_VERSION="+strings.TrimPrefix(version, "v"))
```

Current installer script accepts either `v0.17.0` or `0.17.0` from the environment according to README, and `selfupdate` currently passes the version without the leading `v`.

Repo conventions to follow:

- App layer defines ports; infra implements concrete GitHub/script/network behavior.
- Keep user-facing wording in CLI where possible, but this plan should not expand the activity port refactor.
- Prefer minimal security improvement with tests over a broad self-update rewrite.

## Commands you will need

| Purpose | Command | Expected on success |
|---|---|---|
| Selfupdate infra tests | `go test ./internal/infra/selfupdate` | exit 0, all tests pass |
| App tests | `go test ./internal/app` | exit 0, all tests pass |
| Full tests | `go test ./...` | exit 0, all packages pass |
| Vet | `go vet ./...` | exit 0, no diagnostics |
| Full local gate | `make verify` | exit 0 |

## Scope

**In scope**:

- `internal/app/selfupdate_ports.go` if the port needs version/tag context clarified
- `internal/app/service.go` only if passing a tag instead of trimmed version changes
- `internal/infra/selfupdate/installer.go`
- `internal/infra/selfupdate/installer_test.go`
- `plans/README.md` status update only

**Out of scope**:

- Implementing binary replacement from release assets directly. That is a larger design unless the tagged-script approach is impossible.
- Adding signature/checksum verification.
- Changing README install command.
- Refactoring activity wording.

## Git workflow

- Do not create commits unless explicitly asked.
- If committing later, use a message like `fix(selfupdate): fetch installer from release tag`.

## Steps

### Step 1: Confirm Plan 003 tests exist and pass

Run the selfupdate infra tests added by Plan 003. If `internal/infra/selfupdate/installer_test.go` does not exist, stop and execute Plan 003 first.

**Verify**: `go test ./internal/infra/selfupdate` → exit 0.

### Step 2: Change script URL construction to use the selected tag

In `internal/infra/selfupdate/installer.go`, stop using the `main` URL for default self-update installs. Recommended shape:

- Replace `defaultInstallScriptURL` with a format or helper such as `installScriptURLForVersion(version string) string`.
- For version `v0.18.0` or `0.18.0`, fetch `https://raw.githubusercontent.com/slobbe/appimage-manager/v0.18.0/scripts/install.sh`.
- Preserve `Installer.ScriptURL` as a test/override escape hatch: if `ScriptURL` is non-empty, use it exactly as today; otherwise derive the tagged URL from `version`.
- Pass `AIM_VERSION` to the script as the normalized version expected by existing behavior. Current code strips the leading `v`; keep that unless tests or script behavior show it is wrong.

Do not fetch from `main` unless an explicit `ScriptURL` override is set by tests or future wiring.

**Verify**: `go test ./internal/infra/selfupdate` → tests may fail until updated in Step 3.

### Step 3: Update/add tests for tagged URL behavior

In `internal/infra/selfupdate/installer_test.go`, add or update tests to assert:

- With `Installer{HTTPClient: client}` and version `v0.18.0`, the request path includes `/slobbe/appimage-manager/v0.18.0/scripts/install.sh` equivalent if using a custom transport, or more simply test the helper `installScriptURLForVersion("v0.18.0")` if it is unexported in the same package.
- Version `0.18.0` normalizes to tag `v0.18.0` in the URL.
- Explicit `ScriptURL` still overrides the default; existing tests using `httptest.Server` should continue to work.

If testing the real host URL would require network, do not do that. Test helpers and overrides locally.

**Verify**: `go test ./internal/infra/selfupdate` → all tests pass.

### Step 4: Check app-layer version/tag passing

Inspect `internal/app/service.go` to ensure `s.selfUpdater.Install(ctx, version)` passes the release tag from GitHub, e.g. `v0.18.0`. It currently uses `version := release.TagName` and calls `Install(ctx, version)`. If that is still true, no app-layer change is needed.

If app code now passes a trimmed version, change it back to the release tag or adjust the infra helper to normalize safely.

**Verify**: `go test ./internal/app` → all tests pass.

### Step 5: Run full verification

**Verify**:

- `go test ./...` → all packages pass.
- `go vet ./...` → exit 0.
- `make verify` → exit 0.

## Test plan

- Extend selfupdate infra tests from Plan 003.
- Cover default tagged URL, no-`v` normalization, explicit `ScriptURL` override, success command invocation, and command failure.
- Do not add tests that make real network calls.

## Done criteria

- [ ] Default self-update installer fetches `scripts/install.sh` from the selected release tag, not `main`.
- [ ] Explicit `Installer.ScriptURL` override still works for tests/custom use.
- [ ] Version normalization handles both `vX.Y.Z` and `X.Y.Z`.
- [ ] No real network calls in tests.
- [ ] `go test ./internal/infra/selfupdate`, `go test ./...`, `go vet ./...`, and `make verify` pass.
- [ ] `plans/README.md` status row updated.

## STOP conditions

Stop and report if:

- Plan 003 has not been executed and selfupdate infra tests do not exist.
- Fetching from tags is incompatible with how releases are cut in this repo.
- The install script at historical tags is not expected to support self-update for the selected binary/install layout.
- A robust fix appears to require direct binary replacement, checksum files, or release asset redesign. That should become a separate design plan.
- Any verification fails twice after focused fixes.

## Maintenance notes

This is an incremental supply-chain improvement, not full integrity verification. Future hardening should prefer release-asset checksums/signatures or direct binary replacement, but this plan removes the immediate mutable-branch execution risk with a smaller blast radius.