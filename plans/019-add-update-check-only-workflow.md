# Plan 019: Add a first-class update check-only workflow

> **Executor instructions**: Follow this plan step by step. This is a design/spike plus minimal implementation plan. Run every verification command and confirm expected results. If a STOP condition occurs, stop and report. Update `plans/README.md` when done.
>
> **Drift check (run first)**: `git diff --stat 0cee363..HEAD -- README.md internal/app/service.go internal/app/service_ports.go internal/cli/command/update/command.go internal/cli/command/update/command_test.go`

## Status

- **Priority**: P2
- **Effort**: M
- **Risk**: MED
- **Depends on**: none
- **Category**: direction
- **Planned at**: commit `0cee363`, 2026-06-13

## Why this matters

Automation needs a safe way to discover available AppImage updates without mutating files. Today `aim update` plans updates and then applies them after confirmation; JSON mode auto-confirms, so machine-readable mode is more mutating rather than safer. A check-only mode would let users/scripts inspect pending updates and decide separately whether to apply.

## Current state

Current update request has no mode:

```go
// internal/app/service_ports.go:38-42
type UpdateRequest struct {
    Target       string
    Activity     ActivityReporter
    Confirmation UpdateConfirmation
}
```

Current update behavior applies plans:

```go
// internal/app/service.go:350-374
plans, candidates, err := s.planGitHubUpdates(ctx, req.Target, activity)
// confirmation
for _, plan := range plans {
    if err := s.applyGitHubUpdate(ctx, activity, plan); err != nil { return UpdateResult{}, err }
}
return UpdateResult{Applied: true, Updates: candidates}, nil
```

JSON mode auto-confirms:

```go
// internal/cli/command/update/command.go:51-57
Confirmation: updatePrompter{autoConfirm: rt.Config.JSON}
// internal/cli/command/update/command.go:218-220
if p.autoConfirm { return true, nil }
```

## Commands you will need

| Purpose | Command | Expected on success |
|---|---|---|
| App tests | `go test ./internal/app` | exit 0 |
| Update CLI tests | `go test ./internal/cli/command/update` | exit 0 |
| Full tests | `go test ./...` | exit 0 |
| Vet | `go vet ./...` | exit 0 |
| Full gate | `make verify` | exit 0 |

## Scope

**In scope**:

- `internal/app/service_ports.go`
- `internal/app/service.go`
- `internal/app/service_test.go`
- `internal/cli/command/update/command.go`
- `internal/cli/command/update/command_test.go`
- README update examples if adding a user-facing flag
- `plans/README.md` status update only

**Out of scope**:

- Parallelizing update checks.
- Supporting non-GitHub update sources; see Plan 020.
- Changing default `aim update` apply behavior unless deliberately approved.

## Steps

### Step 1: Choose CLI/API shape

Choose one minimal shape:

- Add `aim update --check` to list candidates without applying.
- Or add `aim update --dry-run` if the project prefers dry-run terminology.

Prefer `--check` because README already says "Check and apply updates" and this mode only checks.

**Verify**: no command; decision recorded in code comments/tests.

### Step 2: Add app-layer check-only mode

Add a field to `UpdateRequest`, for example:

```go
CheckOnly bool
```

In `Update`, after `planGitHubUpdates`, if `CheckOnly` is true, return `UpdateResult{Applied: false, Updates: candidates}` without prompting or calling `applyGitHubUpdate`.

Add app tests proving:

- Check-only with candidates returns candidates and does not call downloader/installer/save.
- Check-only with no candidates returns no updates and does not apply.
- Normal update behavior remains unchanged.

**Verify**: `go test ./internal/app` → exit 0.

### Step 3: Add CLI flag and output

In `internal/cli/command/update/command.go`, add a bool flag `--check`. Set `req.CheckOnly = checkOnly`. In text output, show pending updates without success-applied wording. In JSON output, include enough data for automation: at minimum `applied` and update candidates. If changing JSON shape from only status/action/target/applied, update tests accordingly.

Do not auto-confirm in check-only mode because no apply happens.

**Verify**: `go test ./internal/cli/command/update` → exit 0.

### Step 4: Update docs

Add README examples:

```sh
aim update --check
aim --json update --check
```

Explain that `--check` does not modify installed AppImages.

**Verify**: `go test ./internal/cli/command/update` → still passes.

### Step 5: Run full verification

**Verify**:

- `go test ./...` → exit 0.
- `go vet ./...` → exit 0.
- `make verify` → exit 0.

## Done criteria

- [ ] User-facing check-only update mode exists and is documented.
- [ ] App tests prove check-only does not apply/download/install/save updates.
- [ ] CLI tests cover text and JSON check-only output.
- [ ] Existing default `aim update` behavior remains covered and unchanged unless approved.
- [ ] `go test ./...`, `go vet ./...`, and `make verify` pass.
- [ ] `plans/README.md` status row updated.

## STOP conditions

Stop and report if:

- Choosing JSON output shape requires a breaking schema decision the operator has not approved.
- Check-only behavior conflicts with existing command tests or user expectations.
- You discover update planning itself mutates state.

## Maintenance notes

Future automation features should build on check-only output. Reviewers should verify `--json update --check` is safe for scripts and does not auto-apply.
