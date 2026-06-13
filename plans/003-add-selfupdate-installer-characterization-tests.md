# Plan 003: Add self-update installer characterization tests

> **Executor instructions**: Follow this plan step by step. Run every verification command and confirm the expected result before moving to the next step. If anything in the "STOP conditions" section occurs, stop and report — do not improvise. When done, update the status row for this plan in `plans/README.md`.
>
> **Drift check (run first)**: `git diff --stat 57f5ab0..HEAD -- internal/infra/selfupdate/installer.go internal/cli/command/selfupdate/command_test.go`
> If any in-scope file changed since this plan was written, compare the "Current state" excerpts against the live code before proceeding; on a mismatch, treat it as a STOP condition.

## Status

- **Priority**: P1
- **Effort**: M
- **Risk**: LOW
- **Depends on**: none
- **Category**: tests
- **Planned at**: commit `57f5ab0`, 2026-06-13

## Why this matters

The self-update infra adapter downloads a shell script and executes `sh` with `AIM_VERSION` set. It is a high-risk path, but `go test ./... -cover` reports `github.com/slobbe/appimage-manager/internal/infra/selfupdate coverage: 0.0%`. Before changing self-update behavior in Plan 004, add characterization tests that lock down current behavior and error handling.

## Current state

Relevant files:

- `internal/infra/selfupdate/installer.go` — untested infra adapter that fetches and runs the installer script.
- `internal/cli/command/selfupdate/command_test.go` — existing CLI-level self-update tests; use for style only, not as a substitute for infra tests.
- New file to create: `internal/infra/selfupdate/installer_test.go`.

Current adapter excerpt:

```go
// internal/infra/selfupdate/installer.go:15
const defaultInstallScriptURL = "https://raw.githubusercontent.com/slobbe/appimage-manager/main/scripts/install.sh"
```

```go
// internal/infra/selfupdate/installer.go:29-55
func (i Installer) Install(ctx context.Context, version string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	version = strings.TrimSpace(version)
	if version == "" {
		return errors.New("self-update version is required")
	}

	script, err := i.fetchInstallScript(ctx)
	if err != nil {
		return err
	}
	defer script.Close()

	cmd := exec.CommandContext(ctx, "sh")
	cmd.Stdin = script
	cmd.Env = append(cmd.Environ(), "AIM_VERSION="+strings.TrimPrefix(version, "v"))
	output, err := cmd.CombinedOutput()
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		return fmt.Errorf("run self-update installer: %w: %s", err, strings.TrimSpace(string(output)))
	}

	return nil
}
```

```go
// internal/infra/selfupdate/installer.go:70-82
resp, err := i.httpClient().Do(req)
// ...
if resp.StatusCode < 200 || resp.StatusCode > 299 {
	resp.Body.Close()
	return nil, fmt.Errorf("download install script %q: server returned %s", url, resp.Status)
}

return resp.Body, nil
```

Repo conventions to follow:

- Tests use the standard `testing` package, `httptest.Server`, `t.TempDir`, and explicit `t.Fatalf` assertions.
- Keep infra code independent of CLI.
- Prefer small seams over broad rewrites. This plan is primarily test-only; production changes should be minimal and only to make the adapter testable.

## Commands you will need

| Purpose | Command | Expected on success |
|---|---|---|
| Focused selfupdate tests | `go test ./internal/infra/selfupdate` | exit 0, package tests pass |
| Coverage confirmation | `go test ./internal/infra/selfupdate -cover` | exit 0, coverage above 0% |
| Full tests | `go test ./...` | exit 0, all packages pass |
| Vet | `go vet ./...` | exit 0, no diagnostics |
| Full local gate | `make verify` | exit 0 |

## Scope

**In scope**:

- `internal/infra/selfupdate/installer.go` only if a minimal test seam is needed
- `internal/infra/selfupdate/installer_test.go` (create)
- `plans/README.md` status update only

**Out of scope**:

- Changing the self-update strategy away from the script. That is Plan 004.
- Changing CLI self-update prompts/output.
- Adding checksum/signature verification.

## Git workflow

- Do not create commits unless explicitly asked.
- If committing later, use a message like `test(selfupdate): characterize installer adapter`.

## Steps

### Step 1: Add tests for input validation and HTTP errors

Create `internal/infra/selfupdate/installer_test.go`.

Add tests for:

- Empty/whitespace version returns an error containing `version` and does not contact the server.
- A non-2xx server response returns an error containing the status code.
- A canceled context before `Install` returns `context.Canceled`.

Use `httptest.Server` and `Installer{HTTPClient: server.Client(), ScriptURL: server.URL}`.

**Verify**: `go test ./internal/infra/selfupdate` → tests pass or reveal missing testability only for command execution tests.

### Step 2: Add a minimal command-runner seam if needed

Testing `exec.CommandContext(ctx, "sh")` directly can be brittle because it depends on the real shell and PATH. If direct tests are awkward, add a small unexported seam in `installer.go`, for example:

```go
type commandRunner func(ctx context.Context, name string, stdin io.Reader, env []string) ([]byte, error)

type Installer struct {
	HTTPClient *http.Client
	ScriptURL  string
	runCommand commandRunner
}
```

Then have production default call `exec.CommandContext` exactly as today. Keep the exported fields unchanged. If adding an unexported field to `Installer` is enough for tests in the same package, do not add any exported API.

**Verify**: `go test ./internal/infra/selfupdate` → package compiles.

### Step 3: Test successful install behavior

Add a success test that:

- Serves a script body from `httptest.Server`.
- Calls `Installer.Install(ctx, "v0.18.0")`.
- Asserts the command runner receives command name `sh`.
- Asserts stdin contains the served script content.
- Asserts env contains `AIM_VERSION=0.18.0` (trimmed `v`).

If you did not add a runner seam, use a temporary fake `sh` script on `PATH` that records stdin/env to files in `t.TempDir`; restore `PATH` with `t.Setenv`.

**Verify**: `go test ./internal/infra/selfupdate` → all tests pass.

### Step 4: Test command failure and context cancellation during command

Add tests for:

- Runner returns an error and output text; `Install` error includes `run self-update installer` and the output text.
- Context canceled before/during command execution returns `context.Canceled` when applicable.

Do not overfit exact full error strings.

**Verify**: `go test ./internal/infra/selfupdate -cover` → exit 0 and coverage is above 0%.

### Step 5: Run full verification

**Verify**:

- `go test ./...` → all packages pass.
- `go vet ./...` → exit 0.
- `make verify` → exit 0.

## Test plan

- New `internal/infra/selfupdate/installer_test.go` covers validation, non-2xx download, success command invocation, version trimming, command failure output, and context cancellation.
- Existing `internal/cli/command/selfupdate/command_test.go` remains CLI-level only.

## Done criteria

- [ ] `go test ./internal/infra/selfupdate -cover` reports coverage above 0%.
- [ ] Tests prove `AIM_VERSION` strips a leading `v`.
- [ ] Tests prove the install script body is passed to `sh` on stdin.
- [ ] Tests cover non-2xx HTTP response and command failure output.
- [ ] Any production changes are limited to minimal test seams preserving current behavior.
- [ ] `go test ./...`, `go vet ./...`, and `make verify` pass.
- [ ] `plans/README.md` status row updated.

## STOP conditions

Stop and report if:

- You need to change exported app-layer interfaces to test the infra adapter.
- The drift-check excerpts do not match live code.
- Direct command testing would execute downloaded network content or otherwise escape the test temp directory.
- Any verification fails twice after focused fixes.

## Maintenance notes

These tests are a prerequisite for Plan 004. They should characterize current behavior first so the self-update hardening diff can clearly show intentional behavior changes rather than accidental breakage.