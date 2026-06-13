# Plan 018: Design local AppImage inspection as the safe front door to integration

> **Executor instructions**: Follow this plan step by step. This is a design/spike plan, not a build-everything plan. Produce the requested design artifact and prototype/tests only where specified. If anything in the "STOP conditions" section occurs, stop and report — do not improvise. When done, update the status row for this plan in `plans/README.md`.
>
> **Drift check (run first)**: `git diff --stat 0cee363..HEAD -- README.md internal/app/service.go internal/app/service_ports.go internal/cli/command/info/command.go internal/infra/appimage internal/infra/desktop internal/infra/icon`

## Status

- **Priority**: P2
- **Effort**: M
- **Risk**: MED
- **Depends on**: 011 if docs are aligned first; otherwise none
- **Category**: direction
- **Planned at**: commit `0cee363`, 2026-06-13

## Why this matters

The project describes itself as able to inspect AppImages, and README/help currently show `aim info ./Example.AppImage`. The integration workflow already extracts desktop metadata, icon metadata, and embedded update information before installing. A first-class local inspection workflow would let users preview an AppImage before copying files into managed locations, but it must be clear that AppImage extraction executes the AppImage's extraction/update-info modes and is not a static sandboxed parse.

## Current state

- `README.md:3` positions the CLI as install/integrate/inspect/update.
- `README.md:66-67` shows both installed app and local file info examples.
- `internal/app/service.go:218-239` already extracts AppImage metadata during add.
- `internal/app/service.go:700-713` `Info` currently only finds installed app IDs.
- `internal/infra/appimage/extractor.go:57-62` reads embedded update information while extracting.

Current installed-only info implementation:

```go
// internal/app/service.go:700-713
func (s *service) Info(ctx context.Context, req InfoRequest) (InfoResult, error) {
    target := strings.TrimSpace(req.Target)
    if target == "" { return InfoResult{}, errors.New("app target is required") }
    app, err := s.apps.Find(ctx, target)
    if err != nil { return InfoResult{}, err }
    return InfoResult{ID: app.ID, Name: app.Name, Version: app.Version.String(), ExecPath: app.AppImagePath, Source: app.Source, UpdateSource: app.UpdateSource}, nil
}
```

## Commands you will need

| Purpose | Command | Expected on success |
|---|---|---|
| App tests | `go test ./internal/app` | exit 0 |
| Info command tests | `go test ./internal/cli/command/info` | exit 0 if tests exist |
| Full tests | `go test ./...` | exit 0 |
| Architecture | `make test-architecture` | exit 0 |

## Scope

**In scope**:

- A design note under `plans/018-local-inspection-design.md` or within this plan file's completion notes if the operator prefers no new docs
- Optional prototype changes in `internal/app/service.go`, `internal/app/service_ports.go`, and `internal/cli/command/info/command.go`
- App/CLI tests for the chosen minimal API if implementing a slice
- `plans/README.md` status update only

**Out of scope**:

- Full rich preview UI, diffs, or asset browsing.
- Sandboxing AppImage execution.
- Installing/copying files during inspection.
- Network calls during local inspection.

## Steps

### Step 1: Write the design decision

Create a short design note answering:

- Should local inspection be implemented as `aim info <path>`, a new `aim inspect <path>`, or both?
- What fields should local inspection return in text and JSON?
- How will output indicate that the app is not installed?
- What security wording is needed because extraction executes the AppImage?
- Which app-layer ports are reused, and how are repository writes/installers avoided?

**Verify**: no command; design note exists and answers all questions.

### Step 2: Define the minimal app-layer contract

If implementing a prototype, extend `InfoResult` or add a new result field such as `Installed bool`/`TargetKind string`. Ensure local inspection can represent no installed ID and a local exec path. Keep request parsing in CLI and workflow in app.

**Verify**: `go test ./internal/app` → compile passes after contract update.

### Step 3: Prototype local path handling without writes

In `internal/app/service.go`, detect local paths conservatively. Reuse extractor/desktop/icon/update-info logic from add, but do not call installers or `s.apps.Save`. Return metadata only.

Add tests with fakes proving local inspection:

- Calls extractor/discoverers.
- Does not call repository save or installers.
- Returns metadata and `Installed=false` (or chosen equivalent).

**Verify**: `go test ./internal/app` → exit 0.

### Step 4: Update CLI output contract

Update `internal/cli/command/info/command.go` to render the new installed/local distinction in JSON/text. Avoid importing domain/infra.

**Verify**: `go test ./internal/cli/command/info` → exit 0.

### Step 5: Run gates

**Verify**:

- `go test ./...` → exit 0.
- `make test-architecture` → exit 0.
- `make verify` → exit 0.

## Done criteria

- [ ] Design note exists and documents command shape, output shape, security wording, and no-write guarantee.
- [ ] If prototype implemented, local inspection is covered by app tests and performs no repository writes/installers.
- [ ] CLI docs/help/README match chosen behavior.
- [ ] `go test ./...`, `make test-architecture`, and `make verify` pass.
- [ ] `plans/README.md` status row updated.

## STOP conditions

Stop and report if:

- Stakeholder decision is needed between `info <path>` and a new `inspect` command.
- Safe local inspection requires sandboxing or static parsing beyond current architecture.
- Implementing this conflicts with Plan 011's chosen docs-only path; reconcile before coding.

## Maintenance notes

Reviewers should verify inspection is read-only with respect to `aim` state and managed directories. Future richer preview features should build on this contract rather than duplicating add integration logic.
