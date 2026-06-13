# Plan 011: Align the `aim info <path>` contract with real behavior

> **Executor instructions**: Follow this plan step by step. Run every verification command and confirm the expected result before moving to the next step. If anything in the "STOP conditions" section occurs, stop and report — do not improvise. When done, update the status row for this plan in `plans/README.md`.
>
> **Drift check (run first)**: `git diff --stat 0cee363..HEAD -- README.md internal/cli/command/info/command.go internal/app/service.go internal/app/service_test.go internal/cli/command/info/command_test.go`
> If any in-scope file changed since this plan was written, compare the "Current state" excerpts against live code before proceeding; on mismatch, treat it as a STOP condition.

## Status

- **Priority**: P2
- **Effort**: S/M
- **Risk**: LOW for docs-only, MED for implementation
- **Depends on**: none
- **Category**: docs
- **Planned at**: commit `0cee363`, 2026-06-13

## Why this matters

The README and command help promise that `aim info` accepts either an integrated app ID or a local AppImage path. The real app service only calls `s.apps.Find(ctx, target)`, so a local file path is treated as an app ID and likely returns app-not-found. This mismatch makes the CLI feel broken to users following documented examples. This plan intentionally starts with a product decision: either remove the local-path promise now or implement it later through Plan 018.

## Current state

Relevant files:

- `README.md` — user-facing examples.
- `internal/cli/command/info/command.go` — Cobra command usage/help and output formatting.
- `internal/app/service.go` — app-layer `Info` implementation.
- `internal/app/service_test.go` and/or `internal/cli/command/info/command_test.go` — tests to add depending on chosen path.

Current docs/help promise:

```md
<!-- README.md:63-70 -->
aim info example-app
aim info ./Example.AppImage
aim list
aim paths
```

```go
// internal/cli/command/info/command.go:22-24
Use:   "info <appimage-or-path>",
Short: "Get information about an AppImage.",
Long:  "Get information about an integrated AppImage or a local AppImage file.",
```

Current service behavior:

```go
// internal/app/service.go:705-713
target := strings.TrimSpace(req.Target)
if target == "" { return InfoResult{}, errors.New("app target is required") }
app, err := s.apps.Find(ctx, target)
if err != nil { return InfoResult{}, err }
```

Repo conventions to follow:

- CLI layer parses/prints only; app layer owns use-case behavior.
- If implementing local inspection, do not perform integration/removal/update logic in CLI.
- If choosing docs-only, keep the change narrowly scoped to README/help/tests.

## Commands you will need

| Purpose | Command | Expected on success |
|---|---|---|
| Info command tests | `go test ./internal/cli/command/info` | exit 0 |
| App tests | `go test ./internal/app` | exit 0 |
| Full tests | `go test ./...` | exit 0 |
| Vet | `go vet ./...` | exit 0 |
| Full gate | `make verify` | exit 0 |

## Scope

**In scope**:

- `README.md`
- `internal/cli/command/info/command.go`
- `internal/cli/command/info/command_test.go` if adding CLI contract tests
- `internal/app/service.go` and `internal/app/service_test.go` only if implementing local path support now
- `plans/README.md` status update only

**Out of scope**:

- Implementing rich inspect/preview UX beyond parity with the current `info` contract; see Plan 018.
- Changing `list`, `paths`, or `remove` behavior.
- Moving extraction/icon/desktop logic across architecture layers.

## Git workflow

- Do not create commits unless the operator explicitly asks.
- If committing a docs-only fix, use `docs(readme): align info examples with installed apps`.
- If implementing behavior, use `feat(info): inspect local appimage paths`.

## Steps

### Step 1: Decide behavior from code evidence

Choose exactly one path:

- **Preferred if short on time**: docs/help-only correction. Change README and `info` help to say `info <appimage>` / integrated app ID only, and remove `aim info ./Example.AppImage` from README.
- **Preferred if product wants the advertised feature now**: implement local path inspection by reusing app-layer extraction/discovery ports. If choosing this, also read and follow Plan 018, because that plan scopes the design risks.

If no human is available to choose, take the docs/help-only path and leave Plan 018 as the implementation roadmap.

**Verify**: no command; this is a decision gate.

### Step 2A: Docs/help-only fix

Update:

- `README.md`: remove or replace `aim info ./Example.AppImage` with an installed app example.
- `internal/cli/command/info/command.go`: change `Use` to `info <appimage>` or `info <app-id>` and `Long` to mention integrated AppImages only.

Add `internal/cli/command/info/command_test.go` if absent. Cover that the command passes the target string to `app.InfoRequest` and renders JSON/text for installed app info. Use command tests in `internal/cli/command/update/command_test.go` as style guidance, but keep the fake interface limited if Plan 016 has already landed.

**Verify**: `go test ./internal/cli/command/info` → exit 0.

### Step 2B: Implementation path only if explicitly chosen

If implementing local path support, add app-layer behavior that detects a filesystem path and returns an `InfoResult` without repository writes. Use existing extraction/desktop/icon/update-info ports from add integration. Do not put filesystem logic in CLI. Add tests proving:

- Integrated app ID still works.
- Local path returns metadata without calling repository `Save` or installers.
- Missing local file returns a useful error.

**Verify**: `go test ./internal/app ./internal/cli/command/info` → exit 0.

### Step 3: Run full verification

**Verify**:

- `go test ./...` → all packages pass.
- `go vet ./...` → exit 0.
- `make verify` → exit 0.

## Test plan

- Minimum docs/help path: add CLI info command tests for request mapping and JSON/text output.
- Implementation path: add app service tests for local path inspection and no persistence.
- Keep existing README command examples coherent with real behavior.

## Done criteria

- [ ] README and Cobra help no longer promise unsupported behavior, OR local-path behavior is implemented and tested.
- [ ] `go test ./internal/cli/command/info` exits 0.
- [ ] `go test ./...`, `go vet ./...`, and `make verify` exit 0.
- [ ] No CLI layer imports `internal/domain` or `internal/infra`.
- [ ] `plans/README.md` status row updated.

## STOP conditions

Stop and report back if:

- The operator expects full local inspection but the fix is becoming larger than a one-plan implementation.
- Implementing local inspection requires executing AppImages in a way that changes security expectations not covered by tests/docs.
- Plan 018 has already landed and makes this plan obsolete.

## Maintenance notes

If this plan takes the docs-only path, keep Plan 018 as the product implementation plan. Reviewers should ensure README examples match the generated `aim help info` behavior.
