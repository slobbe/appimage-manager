# Plan 005: Rewrite desktop action Exec entries during integration

> **Executor instructions**: Follow this plan step by step. Run every verification command and confirm the expected result before moving to the next step. If anything in the "STOP conditions" section occurs, stop and report — do not improvise. When done, update the status row for this plan in `plans/README.md`.
>
> **Drift check (run first)**: `git diff --stat 57f5ab0..HEAD -- internal/domain/desktop_entry.go internal/domain/desktop_entry_test.go internal/app/service.go internal/app/service_test.go`
> If any in-scope file changed since this plan was written, compare the "Current state" excerpts against the live code before proceeding; on a mismatch, treat it as a STOP condition.

## Status

- **Priority**: P2
- **Effort**: M
- **Risk**: MED
- **Depends on**: none
- **Category**: bug
- **Planned at**: commit `57f5ab0`, 2026-06-13

## Why this matters

During integration, `aim` rewrites the main desktop entry `Exec` to the installed AppImage path, but it preserves `Exec` lines in `[Desktop Action ...]` groups unchanged. AppImage desktop actions often refer to paths in the extracted temporary AppDir or the original AppImage location. After workspace cleanup or source movement, launcher context-menu actions can fail or run stale commands even though the main launcher works.

## Current state

Relevant files:

- `internal/domain/desktop_entry.go` — parses and serializes `.desktop` files.
- `internal/domain/desktop_entry_test.go` — tests preserving fields/groups and currently preserving action `Exec` unchanged.
- `internal/app/service.go` — integration workflow calls `WithExec(...).WithIcon(...)` before desktop entry install.
- `internal/app/service_test.go` — service integration test currently asserts action groups and stale action exec are preserved.

Current parser behavior:

```go
// internal/domain/desktop_entry.go:25-29
// Only keys in the [Desktop Entry] group are mapped to the domain entry. Unknown
// keys are preserved in Fields so callers can later serialize the entry without
// losing metadata they do not understand yet.
func ParseDesktopEntry(content []byte) (DesktopEntry, error) {
```

```go
// internal/domain/desktop_entry.go:60-64
entryLine.group = currentGroup
if !inDesktopEntry {
	lines[i] = entryLine
	continue
}
```

Current rewrite behavior:

```go
// internal/domain/desktop_entry.go:100-103
// WithExec returns a copy of the entry with the Desktop Entry Exec key updated.
func (d DesktopEntry) WithExec(exec string) DesktopEntry {
	return d.withField("Exec", exec)
}
```

```go
// internal/app/service.go:261-264
updatedDesktopEntry := desktopEntry.
	WithExec(installedAppImagePath).
	WithIcon(provisionalApp.ID)
```

Current test expecting stale action exec preservation:

```go
// internal/domain/desktop_entry_test.go:185-188
"[Desktop Action NewWindow]",
"Name=New Window",
"Exec=/tmp/old.AppImage --new-window",
"",
```

Service test also currently asserts this behavior:

```go
// internal/app/service_test.go:90-95
if !strings.Contains(desktopContent, "[Desktop Action NewWindow]") {
	t.Fatalf("desktop content = %q, want action group preserved", desktopContent)
}
if !strings.Contains(desktopContent, "Exec=old-action") {
	t.Fatalf("desktop content = %q, want action Exec preserved", desktopContent)
}
```

Repo conventions to follow:

- Desktop entry parsing/serialization lives in `internal/domain` and must remain pure: no filesystem, CLI, app, or infra imports.
- App workflow should call domain methods; do not put desktop-entry string parsing in `internal/app` or `internal/infra`.
- Tests are standard Go tests with explicit string comparisons.

## Commands you will need

| Purpose | Command | Expected on success |
|---|---|---|
| Domain tests | `go test ./internal/domain` | exit 0, all tests pass |
| App tests | `go test ./internal/app` | exit 0, all tests pass |
| Full tests | `go test ./...` | exit 0, all packages pass |
| Vet | `go vet ./...` | exit 0, no diagnostics |
| Architecture | `make test-architecture` | exit 0, no output |
| Full local gate | `make verify` | exit 0 |

## Scope

**In scope**:

- `internal/domain/desktop_entry.go`
- `internal/domain/desktop_entry_test.go`
- `internal/app/service.go` only if the domain API requires a call-site name change
- `internal/app/service_test.go`
- `plans/README.md` status update only

**Out of scope**:

- Full freedesktop `.desktop` parser rewrite.
- Shell-quoting/argument parser for arbitrary `Exec` commands beyond preserving existing arguments safely enough.
- Removing desktop actions entirely unless preserving/rewrite is impossible; if so, STOP first.
- Any infra filesystem changes.

## Git workflow

- Do not create commits unless explicitly asked.
- If committing later, use a message like `fix(desktop): rewrite action exec entries`.

## Steps

### Step 1: Add failing domain tests for action Exec rewriting

In `internal/domain/desktop_entry_test.go`, update the existing test around lines 141-193 or add a new test that expects:

- Main `[Desktop Entry]` `Exec=/tmp/old.AppImage --old-flag` becomes `Exec=/apps/example.AppImage` (current behavior).
- `[Desktop Action NewWindow]` `Exec=/tmp/old.AppImage --new-window` becomes `Exec=/apps/example.AppImage --new-window`.
- Action group `Name=New Window`, comments, blank lines, and group ordering are preserved.

Also add a test where action `Exec` is just `old-action` with no obvious AppImage path. For this case, prefer replacing the command token with the installed AppImage path and preserving trailing arguments if any. If the desired behavior is ambiguous, STOP and ask.

**Verify**: `go test ./internal/domain` → should fail before implementation.

### Step 2: Track keys in Desktop Action groups

In `internal/domain/desktop_entry.go`, adjust parsing so `desktopEntryLine.key` is populated for key/value lines in all groups, not only `[Desktop Entry]`. Keep `DesktopEntry.Fields`, `Name`, `Exec`, `Icon`, and `Version` mapped only from the main `[Desktop Entry]` group.

A safe shape:

- For any non-comment, non-group line with `key=value`, parse `key` and set `entryLine.key = key`.
- If `inDesktopEntry`, also put it in `fields` as today.
- If not `inDesktopEntry`, do not add it to `fields`, but keep `line.group` and `line.key` for serialization transforms.

Be careful not to start rejecting non-Desktop-Entry malformed lines unless current behavior already rejects them. If parsing keys outside the main group would newly reject real-world files, STOP and report.

**Verify**: `go test ./internal/domain` → may still fail until Step 3.

### Step 3: Rewrite action Exec lines when WithExec is called

In `desktop_entry.go`, extend `WithExec`/`withField` behavior so that updating the main `Exec` also updates `Exec` keys in groups whose name starts with `Desktop Action `.

Recommended helper behavior:

- Detect action lines with `strings.HasPrefix(line.group, "Desktop Action ") && line.key == "Exec"`.
- Preserve arguments when possible. For raw value `Exec=/tmp/old.AppImage --new-window`, produce `Exec=<newExec> --new-window`.
- Keep the main `Desktop Entry` line update exactly as current behavior.

Simple argument preservation is acceptable if it handles the current common case: split the old value on the first whitespace and append the suffix to the new executable. Do not attempt a full shell parser in this plan.

**Verify**: `go test ./internal/domain` → all domain tests pass.

### Step 4: Update app-level integration expectations

In `internal/app/service_test.go`, update `TestServiceAddIntegratesLocalAppImage` so it still asserts `[Desktop Action NewWindow]` is preserved, but no longer expects `Exec=old-action`. Instead assert the installed desktop content contains the action group and an action exec using `/library/example-app.AppImage`.

If the fake desktop content currently uses `Exec=old-action`, update the expectation according to the domain behavior chosen in Step 3.

**Verify**: `go test ./internal/app` → all app tests pass.

### Step 5: Run full verification

**Verify**:

- `go test ./...` → all packages pass.
- `go vet ./...` → exit 0.
- `make test-architecture` → exit 0.
- `make verify` → exit 0.

## Test plan

- Domain tests cover action group Exec rewrite with argument preservation and group/comment preservation.
- App service test confirms installed desktop entry content rewrites action Exec during real integration workflow.
- Existing domain tests for main `Exec`/`Icon` update and missing field insertion continue to pass.

## Done criteria

- [ ] `WithExec` updates main desktop entry `Exec` and desktop action `Exec` lines.
- [ ] Action groups, names, comments, ordering, and blank lines are preserved.
- [ ] Domain tests cover action Exec rewrite and pass.
- [ ] App integration test no longer expects stale action `Exec` preservation.
- [ ] `go test ./internal/domain`, `go test ./internal/app`, `go test ./...`, `go vet ./...`, `make test-architecture`, and `make verify` pass.
- [ ] `plans/README.md` status row updated.

## STOP conditions

Stop and report if:

- The live desktop parser differs from the excerpts after drift check.
- Correctly preserving action semantics appears to require full desktop `Exec` token parsing/quoting.
- Rewriting action `Exec` breaks existing tests in a way that suggests action commands are not AppImage invocations.
- A fix would require importing app/infra/cli packages into `internal/domain`.
- Any verification fails twice after focused fixes.

## Maintenance notes

This plan intentionally handles the common action command shape without implementing a full freedesktop Exec parser. Reviewers should inspect argument preservation carefully. If later bugs appear around quoted executable paths or field codes, add a dedicated parser plan rather than expanding this patch ad hoc.