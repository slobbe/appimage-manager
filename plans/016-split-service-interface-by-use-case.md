# Plan 016: Split the all-in-one app service boundary by CLI use case

> **Executor instructions**: Follow this plan step by step. Run every verification command and confirm the expected result before moving to the next step. If anything in the "STOP conditions" section occurs, stop and report — do not improvise. When done, update the status row for this plan in `plans/README.md`.
>
> **Drift check (run first)**: `git diff --stat 0cee363..HEAD -- internal/app/service_ports.go internal/cli cmd/aim internal/cli/command`

## Status

- **Priority**: P3
- **Effort**: M
- **Risk**: MED
- **Depends on**: none
- **Category**: tech-debt
- **Planned at**: commit `0cee363`, 2026-06-13

## Why this matters

`internal/app.Service` exposes every CLI use case in one interface. Command constructors accept the full interface even when they need one method, so command tests must implement no-op methods for unrelated behavior. Adding or changing any use-case method ripples through unrelated tests and makes refactors noisy. Smaller use-case interfaces improve testability while keeping the concrete service unchanged.

## Current state

Relevant files:

- `internal/app/service_ports.go` — defines the broad `Service` interface.
- `internal/cli/command/*/command.go` — command constructors take `app.Service`.
- `internal/cli/command/*/*_test.go` — fake services implement many no-op methods.
- `cmd/aim/main.go` and `internal/cli/root.go` — wire concrete service to commands.

Current broad interface:

```go
// internal/app/service_ports.go:9-19
type Service interface {
    Add(ctx context.Context, req AddRequest) (AddResult, error)
    Remove(ctx context.Context, req RemoveRequest) error
    Update(ctx context.Context, req UpdateRequest) (UpdateResult, error)
    SetUpdateSource(ctx context.Context, req SetUpdateSourceRequest) (SetUpdateSourceResult, error)
    UnsetUpdateSource(ctx context.Context, req UnsetUpdateSourceRequest) error
    List(ctx context.Context, req ListRequest) (ListResult, error)
    Info(ctx context.Context, req InfoRequest) (InfoResult, error)
    SelfUpdate(ctx context.Context, req SelfUpdateRequest) (SelfUpdateResult, error)
    Paths(ctx context.Context, req PathsRequest) (PathsResult, error)
}
```

Fake boilerplate examples exist in `internal/cli/command/add/command_github_test.go:50-90`, `internal/cli/command/update/command_test.go:289-351`, and `internal/cli/command/selfupdate/command_test.go:67-104`.

## Commands you will need

| Purpose | Command | Expected on success |
|---|---|---|
| CLI command tests | `go test ./internal/cli/...` | exit 0 |
| App tests | `go test ./internal/app` | exit 0 |
| Full tests | `go test ./...` | exit 0 |
| Architecture | `make test-architecture` | exit 0 |
| Full gate | `make verify` | exit 0 |

## Scope

**In scope**:

- `internal/app/service_ports.go` for small use-case interfaces, or command-local interfaces in `internal/cli/command/*`
- `internal/cli/root.go`
- `internal/cli/command/*/command.go`
- Corresponding command tests/fakes
- `plans/README.md` status update only

**Out of scope**:

- Changing concrete service behavior in `internal/app/service.go`.
- Moving ports to infra or CLI in a way that violates `AGENTS.md`.
- Renaming request/result types.

## Steps

### Step 1: Choose interface location

Prefer app-layer use-case interfaces because they describe app capabilities and keep CLI depending only on app. Example:

```go
type Adder interface { Add(context.Context, AddRequest) (AddResult, error) }
type Remover interface { Remove(context.Context, RemoveRequest) error }
type Updater interface {
    Update(context.Context, UpdateRequest) (UpdateResult, error)
    SetUpdateSource(context.Context, SetUpdateSourceRequest) (SetUpdateSourceResult, error)
    UnsetUpdateSource(context.Context, UnsetUpdateSourceRequest) error
}
```

Keep the existing `Service` interface as the aggregate for root wiring if useful.

**Verify**: `go test ./internal/app` → exit 0.

### Step 2: Update command constructors one package at a time

For each command package, change constructor signatures to accept the narrow interface:

- `add.NewCommand(..., app.Adder)`
- `remove.NewCommand(..., app.Remover)`
- `update.NewCommand(..., app.Updater)` because it uses update and source methods
- `list.NewCommand(..., app.Lister)`
- `info.NewCommand(..., app.Infoer)` or `app.Informer`
- `selfupdate.NewCommand(..., app.SelfUpdater)`
- `paths.NewCommand(..., app.Pathser)` or better-named equivalent

Use clear names; if a name feels awkward, prefer command-local unexported interfaces over bad exported names.

**Verify**: `go test ./internal/cli/...` → fix compile errors package by package.

### Step 3: Simplify command test fakes

Remove no-op methods from test fakes that are no longer required. Each fake should implement only the methods that command calls.

**Verify**: `go test ./internal/cli/...` → exit 0.

### Step 4: Confirm root wiring remains simple

`cmd/aim/main.go` should still construct one concrete service and pass it into `cli.Execute`. `internal/cli/root.go` can pass that same service to all commands because the concrete service implements every narrow interface implicitly.

**Verify**: `go test ./...` → exit 0.

### Step 5: Run architecture and full gates

**Verify**:

- `make test-architecture` → exit 0.
- `make verify` → exit 0.

## Done criteria

- [ ] Command constructors no longer require the full `app.Service` unless they truly use all methods.
- [ ] Command test fakes contain only relevant methods.
- [ ] Concrete app service still wires through root without adapters.
- [ ] `go test ./internal/cli/...`, `go test ./...`, `make test-architecture`, and `make verify` pass.
- [ ] No dependency rule from `AGENTS.md` is violated.
- [ ] `plans/README.md` status row updated.

## STOP conditions

Stop and report if:

- Interface splitting causes import cycles.
- Naming exported interfaces becomes unclear; propose command-local interfaces instead of forcing poor names.
- Behavior changes are needed to make tests pass.

## Maintenance notes

Future command packages should depend on the smallest interface they need. Reviewers should watch for new tests reintroducing broad fake services with unrelated no-op methods.
