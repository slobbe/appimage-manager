# Plan 006: Upgrade Go patch version to fix reachable standard-library vulnerabilities

> **Executor instructions**: Follow this plan step by step. Run every verification command and confirm the expected result before moving to the next step. If anything in the "STOP conditions" section occurs, stop and report — do not improvise. When done, update the status row for this plan in `plans/README.md`.
>
> **Drift check (run first)**: `git diff --stat 0cee363..HEAD -- go.mod go.sum Makefile .github/workflows/ci.yml .github/workflows/release.yml internal/infra/download/downloader.go internal/cli/command/update/command.go`
> If any in-scope file changed since this plan was written, compare the "Current state" excerpts against live code before proceeding; on mismatch, treat it as a STOP condition.

## Status

- **Priority**: P1
- **Effort**: S
- **Risk**: LOW
- **Depends on**: none
- **Category**: security
- **Planned at**: commit `0cee363`, 2026-06-13

## Why this matters

`make audit` currently fails because `govulncheck` reports two reachable vulnerabilities in the Go `1.25.10` standard library. Released `aim` binaries include the Go runtime/standard library they were built with, so this is not just a developer-machine warning. The expected fix is a patch-level Go upgrade to a version containing the fixed `net/textproto` and `crypto/x509` packages, without changing application behavior.

## Current state

Relevant files:

- `go.mod` — declares the Go toolchain version used by `actions/setup-go` and local Go commands.
- `.github/workflows/ci.yml` — uses `go-version-file: go.mod` and runs `make audit`.
- `.github/workflows/release.yml` — also uses `go-version-file: go.mod` for release builds.
- `internal/infra/download/downloader.go` — `govulncheck` reachable trace for `net/textproto` via HTTP body reads.
- `internal/cli/command/update/command.go` — `govulncheck` reachable trace for `crypto/x509`.

Current version:

```go
// go.mod:1-4
module github.com/slobbe/appimage-manager

go 1.25.10
```

Audit output observed at plan time:

```text
Vulnerability #1: GO-2026-5039 ... net/textproto ... Found in: net/textproto@go1.25.10 ... Fixed in: net/textproto@go1.25.11
Example trace: internal/infra/download/downloader.go:110:25: download.copyWithProgress calls http.body.Read

Vulnerability #2: GO-2026-5037 ... crypto/x509 ... Found in: crypto/x509@go1.25.10 ... Fixed in: crypto/x509@go1.25.11
Example trace: internal/cli/command/update/command.go:235:14: update.updatePrompter.ConfirmUpdates calls fmt.Fprintf...
```

Repo conventions to follow:

- Keep this as a focused build/security change; do not refactor application code to silence `govulncheck` traces.
- CI reads the Go version from `go.mod`; prefer a single version source.
- If committing later, use Conventional Commit style such as `build: upgrade go patch version`.

## Commands you will need

| Purpose | Command | Expected on success |
|---|---|---|
| Focused audit | `make audit` | exit 0; `govulncheck` reports no reachable vulnerabilities and `shellcheck scripts/*.sh` passes |
| Tests | `go test ./...` | exit 0 |
| Vet | `go vet ./...` | exit 0, no diagnostics |
| Architecture | `make test-architecture` | exit 0, no grep output |
| Full gate | `make verify` | exit 0 |

## Scope

**In scope**:

- `go.mod`
- `go.sum` only if the Go command updates it as part of the patch upgrade
- CI/release workflow Go-version references only if required by the chosen Go version
- `plans/README.md` status update only

**Out of scope**:

- Editing `internal/infra/download/downloader.go` or `internal/cli/command/update/command.go` just to hide vulnerability traces.
- Upgrading application dependencies unless the Go tool requires metadata updates.
- Pinning audit/release tools; that is Plan 017.
- Changing installer scripts or release logic.

## Git workflow

- Do not create commits unless the operator explicitly asks.
- If committing later, use a message like `build: upgrade go patch version`.

## Steps

### Step 1: Confirm the current vulnerability baseline

Run `make audit` before changing anything. It should fail with reachable standard-library vulnerabilities for Go `1.25.10`.

**Verify**: `make audit` → fails before the fix with `GO-2026-5039` and `GO-2026-5037`. If it already passes, STOP and report; this plan may have been fixed independently.

### Step 2: Upgrade the Go patch version

Edit `go.mod` and change:

```go
go 1.25.10
```

to the minimum fixed patch reported by `govulncheck`:

```go
go 1.25.11
```

If `go1.25.11` is unavailable in the execution environment, use the latest available Go `1.25.x` patch that is greater than or equal to `1.25.11`. Do not jump to a new minor/major release unless the operator approves.

**Verify**: `go version` → reports `go1.25.11` or newer compatible `go1.25.x`. If local Go is still `1.25.10`, STOP and ask the operator to install/select the fixed toolchain rather than editing code around the vulnerability.

### Step 3: Refresh module metadata only if necessary

Run `go mod tidy` only if `go test` or `go list` reports module metadata drift after the `go.mod` change. If `go mod tidy` changes `go.sum`, inspect the diff and ensure it only reflects normal module metadata changes.

**Verify**: `go test ./...` → exit 0.

### Step 4: Re-run audit and verification

Run the audit and normal gates.

**Verify**:

- `make audit` → exit 0. It should run `govulncheck ./...` with no reachable vulnerabilities and then `shellcheck scripts/*.sh` with no errors.
- `go vet ./...` → exit 0.
- `make test-architecture` → exit 0.
- `make verify` → exit 0.

## Test plan

No new unit tests are required because this is a toolchain patch upgrade. The regression guard is `make audit`, which must pass after the upgrade.

## Done criteria

- [ ] `go.mod` declares Go `1.25.11` or newer compatible fixed `1.25.x` patch.
- [ ] `make audit` exits 0.
- [ ] `go test ./...` exits 0.
- [ ] `go vet ./...` exits 0.
- [ ] `make test-architecture` exits 0.
- [ ] `make verify` exits 0.
- [ ] No application source files were changed.
- [ ] `plans/README.md` status row updated.

## STOP conditions

Stop and report back if:

- The local/CI environment cannot run the fixed Go patch version.
- `govulncheck` still reports reachable vulnerabilities after using the fixed Go patch.
- The fix appears to require application code changes instead of a toolchain patch upgrade.
- `go mod tidy` attempts broad dependency changes unrelated to the Go patch.

## Maintenance notes

Future Go patch vulnerabilities should be handled similarly: update the Go patch version, verify with `govulncheck`, and keep the change focused. Reviewers should confirm release workflow uses the same fixed version through `go-version-file: go.mod`.
