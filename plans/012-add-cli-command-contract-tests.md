# Plan 012: Add contract tests for untested user-facing CLI commands

> **Executor instructions**: Follow this plan step by step. Run every verification command and confirm the expected result before moving to the next step. If anything in the "STOP conditions" section occurs, stop and report — do not improvise. When done, update the status row for this plan in `plans/README.md`.
>
> **Drift check (run first)**: `git diff --stat 0cee363..HEAD -- internal/cli/command/info internal/cli/command/list internal/cli/command/paths internal/cli/command/remove internal/app/service_ports.go`

## Status

- **Priority**: P2
- **Effort**: M
- **Risk**: LOW
- **Depends on**: none
- **Category**: tests
- **Planned at**: commit `0cee363`, 2026-06-13

## Why this matters

`info`, `list`, `paths`, and `remove` are public CLI contracts, but their command packages currently report `0.0%` test coverage. Regressions in argument validation, JSON shape, text formatting, activity wiring, or app request mapping can ship while the global coverage gate still passes. This plan adds focused command tests without changing behavior.

## Current state

Relevant command files:

- `internal/cli/command/info/command.go` — formats app info and JSON shape.
- `internal/cli/command/list/command.go` — maps `app.ListResult` to table/JSON.
- `internal/cli/command/paths/command.go` — prints config/storage/cache/desktop/icon paths.
- `internal/cli/command/remove/command.go` — maps remove args, activity, success output.

Coverage observed at plan time:

```text
github.com/slobbe/appimage-manager/internal/cli/command/info    coverage: 0.0% of statements
github.com/slobbe/appimage-manager/internal/cli/command/list    coverage: 0.0% of statements
github.com/slobbe/appimage-manager/internal/cli/command/paths   coverage: 0.0% of statements
github.com/slobbe/appimage-manager/internal/cli/command/remove  coverage: 0.0% of statements
```

Existing test style examples:

- `internal/cli/command/update/command_test.go` — command execution with fake service and buffers.
- `internal/cli/command/selfupdate/command_test.go` — command request mapping and output tests.
- `internal/cli/command/add/command_github_test.go` — flag/request tests.

## Commands you will need

| Purpose | Command | Expected on success |
|---|---|---|
| Focused command tests | `go test ./internal/cli/command/info ./internal/cli/command/list ./internal/cli/command/paths ./internal/cli/command/remove` | exit 0 |
| Full tests | `go test ./...` | exit 0 |
| Coverage spot check | `go test ./internal/cli/command/info ./internal/cli/command/list ./internal/cli/command/paths ./internal/cli/command/remove -cover` | each package > 0% |
| Vet | `go vet ./...` | exit 0 |
| Full gate | `make verify` | exit 0 |

## Scope

**In scope**:

- Create/update `internal/cli/command/info/command_test.go`
- Create/update `internal/cli/command/list/command_test.go`
- Create/update `internal/cli/command/paths/command_test.go`
- Create/update `internal/cli/command/remove/command_test.go`
- Test-only helper types in those packages
- `plans/README.md` status update only

**Out of scope**:

- Changing command behavior except to fix defects exposed by tests; if behavior is wrong, STOP and report unless the fix is trivial and within the command package.
- Broad service interface refactor; see Plan 016.
- Snapshot tests of ANSI color exactness beyond stable substrings.

## Steps

### Step 1: Add list command tests

In `internal/cli/command/list/command_test.go`, test:

- No args calls `service.List` with `app.ListRequest{}`.
- Text output contains headers and expected rows.
- JSON mode outputs `{"items":[...]}` with `id`, `name`, `version`.
- Service error is returned.

**Verify**: `go test ./internal/cli/command/list` → exit 0.

### Step 2: Add paths command tests

In `internal/cli/command/paths/command_test.go`, test:

- No args calls `service.Paths`.
- Text output includes config file, appimage dir, cache dir, desktop dir, icon dir labels/values.
- JSON mode includes the documented fields.
- Service error is returned.

**Verify**: `go test ./internal/cli/command/paths` → exit 0.

### Step 3: Add remove command tests

In `internal/cli/command/remove/command_test.go`, test:

- Exactly one arg is required.
- Command maps arg to `app.RemoveRequest{Name: arg}`.
- JSON mode suppresses activity noise and emits status/action/name.
- Service error is returned.

**Verify**: `go test ./internal/cli/command/remove` → exit 0.

### Step 4: Add info command tests

In `internal/cli/command/info/command_test.go`, test:

- Exactly one arg is required.
- Command maps arg to `app.InfoRequest{Target: arg}`.
- Text output contains name/version/source fields for a representative result.
- JSON output includes `name`, `version`, `exec_path`, `source`, and `update_source` fields.
- Service error is returned.

Coordinate with Plan 011 if it changes help text or local-path behavior.

**Verify**: `go test ./internal/cli/command/info` → exit 0.

### Step 5: Run coverage and full verification

**Verify**:

- `go test ./internal/cli/command/info ./internal/cli/command/list ./internal/cli/command/paths ./internal/cli/command/remove -cover` → all four packages report > 0%.
- `go test ./...` → exit 0.
- `go vet ./...` → exit 0.
- `make verify` → exit 0.

## Done criteria

- [ ] Tests exist for `info`, `list`, `paths`, and `remove` command packages.
- [ ] Each package has request mapping, JSON/text output, arg validation, and error propagation coverage where applicable.
- [ ] Focused command coverage command shows each package > 0%.
- [ ] `go test ./...`, `go vet ./...`, and `make verify` pass.
- [ ] Production behavior is unchanged unless a small command-local bug was discovered and documented.
- [ ] `plans/README.md` status row updated.

## STOP conditions

Stop and report if:

- Tests reveal a behavior bug requiring app-layer changes.
- Plan 016 has changed command constructor interfaces; adapt tests to the new small interfaces, but stop if both approaches conflict.
- Exact ANSI output makes tests brittle; switch to stable substring assertions rather than broad behavior changes.

## Maintenance notes

These tests should serve as patterns for future command packages. Reviewers should reject tests that only execute commands without asserting request mapping or output contracts.
