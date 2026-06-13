# Plan 014: Retire or relocate the unused `DummyService`

> **Executor instructions**: Follow this plan step by step. Run every verification command and confirm the expected result before moving to the next step. If anything in the "STOP conditions" section occurs, stop and report — do not improvise. When done, update the status row for this plan in `plans/README.md`.
>
> **Drift check (run first)**: `git diff --stat 0cee363..HEAD -- internal/app/dummy.go internal/app cmd/aim internal/cli`

## Status

- **Priority**: P2
- **Effort**: S
- **Risk**: LOW if unused, MED if hidden demo use exists
- **Depends on**: none
- **Category**: tech-debt
- **Planned at**: commit `0cee363`, 2026-06-13

## Why this matters

`internal/app/dummy.go` contains a 351-line alternate app service with simulated workflows and hard-coded data. It is exported as `NewDummyService`, but the composition root wires `app.NewService`, and repo search found no callers. Keeping a parallel workflow implementation in the app layer creates search noise, can drift from real semantics, and reinforces presentation wording inside app-layer code.

## Current state

Relevant files:

- `internal/app/dummy.go` — exported dummy service and simulated workflows.
- `cmd/aim/main.go` — real composition root; uses `app.NewService`.
- `internal/app/service_ports.go` — `Service` interface implemented by both real and dummy services.

Evidence:

```go
// internal/app/dummy.go:12-20
type DummyService struct { config Config }
func NewDummyService(config Config) *DummyService { return &DummyService{config: config} }
```

```go
// cmd/aim/main.go:56-73
service, err := app.NewService(app.ServiceDeps{ ... })
```

A repo search at plan time found `NewDummyService` and `DummyService` only in `internal/app/dummy.go`.

## Commands you will need

| Purpose | Command | Expected on success |
|---|---|---|
| Search callers | `grep -R "NewDummyService\|DummyService" . --include='*.go'` | only intentional references remain, or no matches if deleted |
| App tests | `go test ./internal/app` | exit 0 |
| Full tests | `go test ./...` | exit 0 |
| Vet | `go vet ./...` | exit 0 |
| Architecture | `make test-architecture` | exit 0 |
| Full gate | `make verify` | exit 0 |

## Scope

**In scope**:

- `internal/app/dummy.go`
- Tests or docs only if they explicitly reference `DummyService`
- `plans/README.md` status update only

**Out of scope**:

- Changing real `app.Service` behavior.
- Replacing command tests with `DummyService`; use targeted fakes instead.
- Adding a demo mode unless the operator explicitly wants one.

## Steps

### Step 1: Confirm no callers

Run a repo search for `NewDummyService` and `DummyService`. If there are callers outside `internal/app/dummy.go`, STOP and report with the paths; this plan needs a relocate/demo decision instead of deletion.

**Verify**: `grep -R "NewDummyService\|DummyService" . --include='*.go'` → only `internal/app/dummy.go` matches.

### Step 2: Delete the unused dummy service

Remove `internal/app/dummy.go`. Do not replace it with another app-layer fake. Existing command tests should use local fakes; Plan 012 and Plan 016 cover test ergonomics.

**Verify**: `go test ./internal/app` → exit 0.

### Step 3: Run full verification and search again

**Verify**:

- `grep -R "NewDummyService\|DummyService" . --include='*.go'` → no matches.
- `go test ./...` → exit 0.
- `go vet ./...` → exit 0.
- `make test-architecture` → exit 0.
- `make verify` → exit 0.

## Done criteria

- [ ] `internal/app/dummy.go` is removed, or moved to a clearly documented non-production/demo package if hidden caller evidence was found and approved.
- [ ] No production code references `DummyService`.
- [ ] `go test ./...`, `go vet ./...`, `make test-architecture`, and `make verify` pass.
- [ ] `plans/README.md` status row updated.

## STOP conditions

Stop and report if:

- Any caller outside `internal/app/dummy.go` exists.
- The operator wants to keep a demo/simulation mode; ask where it should live before moving it.
- Deleting the file reveals tests depending on dummy semantics.

## Maintenance notes

Use narrow test fakes instead of production dummy services. If a demo mode is later needed, wire it explicitly from `cmd/aim` or a separate command and keep it out of core app workflows.
