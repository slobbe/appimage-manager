# Plan 010: Resolve AppImage-root absolute desktop icon paths

> **Executor instructions**: Follow this plan step by step. Run every verification command and confirm the expected result before moving to the next step. If anything in the "STOP conditions" section occurs, stop and report — do not improvise. When done, update the status row for this plan in `plans/README.md`.
>
> **Drift check (run first)**: `git diff --stat 0cee363..HEAD -- internal/infra/icon/discovery.go internal/infra/icon/discovery_test.go`
> If any in-scope file changed since this plan was written, compare the "Current state" excerpts against live code before proceeding; on mismatch, treat it as a STOP condition.

## Status

- **Priority**: P2
- **Effort**: S/M
- **Risk**: LOW
- **Depends on**: none
- **Category**: bug
- **Planned at**: commit `0cee363`, 2026-06-13

## Why this matters

Desktop entries inside AppImages can use `Icon=/usr/share/icons/...` to refer to a path inside the extracted AppImage filesystem. The current icon discoverer treats any absolute icon path as a host absolute path and rejects it unless it is already under the temporary extraction root. That blocks valid AppImages whose desktop metadata uses common root-absolute paths.

## Current state

Relevant files:

- `internal/infra/icon/discovery.go` — resolves icon names/paths under extracted AppImage roots.
- `internal/infra/icon/discovery_test.go` — focused icon discovery tests.

Contract comment already says absolute paths can be inside the extracted root:

```go
// internal/infra/icon/discovery.go:33-37
// Discover finds the best icon under rootDir for iconName.
//
// iconName is usually the Icon value from the desktop entry. It can be an icon
// name without an extension, a relative path, or an absolute path inside the
// extracted AppImage root.
```

Current implementation resolves host absolute paths only:

```go
// internal/infra/icon/discovery.go:70-87
func resolveExplicitIconPath(rootDir string, iconName string) (string, bool, error) {
    iconName = strings.TrimSpace(iconName)
    if iconName == "" || !filepath.IsAbs(iconName) {
        return "", false, nil
    }

    path, err := filepath.Abs(iconName)
    // ...
    inside, err := pathInside(rootDir, path)
    if !inside {
        return "", false, fmt.Errorf("icon path %q is outside extracted root %q", path, rootDir)
    }
```

Current tests cover absolute host paths already inside `rootDir` and outside `rootDir`:

```go
// internal/infra/icon/discovery_test.go:12-42
func TestDiscovererUsesAbsoluteIconPathInsideRoot(t *testing.T) { ... }
func TestDiscovererRejectsAbsoluteIconPathOutsideRoot(t *testing.T) { ... }
```

Repo conventions to follow:

- Keep icon discovery in infra; do not move path logic into domain/app.
- Use `t.TempDir`, helper `writeIcon`, and plain `testing` style already in `discovery_test.go`.
- Error messages should be stable enough for substring assertions, not exact full-string matching.

## Commands you will need

| Purpose | Command | Expected on success |
|---|---|---|
| Focused icon tests | `go test ./internal/infra/icon` | exit 0 |
| Full tests | `go test ./...` | exit 0 |
| Vet | `go vet ./...` | exit 0 |
| Full gate | `make verify` | exit 0 |

## Scope

**In scope**:

- `internal/infra/icon/discovery.go`
- `internal/infra/icon/discovery_test.go`
- `plans/README.md` status update only

**Out of scope**:

- Changing desktop entry parsing.
- Changing icon installation paths/names.
- Adding support for new icon file extensions.
- Changing `.DirIcon` fallback behavior.

## Git workflow

- Do not create commits unless the operator explicitly asks.
- If committing later, use a message like `fix(icon): resolve root-absolute desktop icon paths`.

## Steps

### Step 1: Add a failing root-absolute icon path test

In `internal/infra/icon/discovery_test.go`, add a test like `TestDiscovererResolvesDesktopAbsoluteIconPathInsideRoot`:

1. Create `root := t.TempDir()`.
2. Create an icon at `filepath.Join(root, "usr", "share", "icons", "hicolor", "256x256", "apps", "example.png")` using `writeIcon`.
3. Call `Discover(context.Background(), root, "/usr/share/icons/hicolor/256x256/apps/example.png")`.
4. Assert returned `file.Path` equals the path under `root`.

**Verify**: `go test ./internal/infra/icon` → new test should fail before implementation with an outside-root error or missing icon behavior.

### Step 2: Resolve desktop root-absolute paths under `rootDir`

In `resolveExplicitIconPath`, when `iconName` is absolute, interpret it as a path rooted at the extracted AppImage root first:

```go
candidate := filepath.Join(rootDir, strings.TrimPrefix(filepath.Clean(iconName), string(filepath.Separator)))
```

Then validate `candidate` with `pathInside(rootDir, candidate)`, `os.Stat`, directory rejection, and supported extension checks.

Important: preserve the existing test that passes a host absolute path already under `rootDir`. One way is:

1. If `iconName` is absolute and already inside `rootDir`, keep using it as-is.
2. Otherwise, treat it as AppImage-root absolute by joining it under `rootDir`.
3. Reject only if the resolved candidate escapes `rootDir` or does not exist / unsupported extension.

**Verify**: `go test ./internal/infra/icon` → all icon tests pass.

### Step 3: Add traversal and outside-root guard tests

Add tests for root-absolute values that try to escape or resolve weirdly, such as `"/../../outside.png"` or an absolute path with unsupported extension under the extracted root. The expected behavior is an error and no host path access outside `rootDir`.

Do not assert full error strings; use stable substrings like `outside extracted root`, `stat icon path`, or `unsupported extension` depending on the chosen implementation.

**Verify**: `go test ./internal/infra/icon` → all tests pass.

### Step 4: Run full verification

**Verify**:

- `go test ./internal/infra/icon` → exit 0.
- `go test ./...` → all packages pass.
- `go vet ./...` → exit 0.
- `make verify` → exit 0.

## Test plan

- Add root-absolute desktop icon path test.
- Add traversal/outside-root guard coverage.
- Keep existing tests for name-only, relative path, `.DirIcon`, host absolute inside root, and host absolute outside root passing.

## Done criteria

- [ ] `Icon=/usr/share/.../example.png` resolves to `<extracted-root>/usr/share/.../example.png` when present.
- [ ] Existing host-absolute-under-root behavior still works.
- [ ] Host paths outside `rootDir` are still rejected or not accidentally read.
- [ ] `go test ./internal/infra/icon` exits 0.
- [ ] `go test ./...`, `go vet ./...`, and `make verify` exit 0.
- [ ] `plans/README.md` status row updated.

## STOP conditions

Stop and report back if:

- The current `pathInside` implementation cannot safely validate cleaned root-absolute paths.
- Fixing this requires changing desktop entry parsing or app-layer contracts.
- Existing outside-root security tests must be weakened to pass.
- In-scope code has drifted from the excerpts.

## Maintenance notes

Reviewers should scrutinize path-cleaning and traversal handling. Future icon discovery changes must keep the distinction clear between host absolute paths and AppImage-root absolute paths from desktop metadata.
