# Plan 013: Cover config loading and XDG path resolution

> **Executor instructions**: Follow this plan step by step. Run every verification command and confirm the expected result before moving to the next step. If anything in the "STOP conditions" section occurs, stop and report — do not improvise. When done, update the status row for this plan in `plans/README.md`.
>
> **Drift check (run first)**: `git diff --stat 0cee363..HEAD -- internal/infra/config/loader.go internal/infra/xdg/dirs.go internal/infra/config internal/infra/xdg`

## Status

- **Priority**: P2
- **Effort**: S/M
- **Risk**: LOW
- **Depends on**: none
- **Category**: tests
- **Planned at**: commit `0cee363`, 2026-06-13

## Why this matters

Config and XDG path resolution decide where `aim` reads configuration and writes managed AppImages, desktop entries, icons, cache, and state. Mistakes here can send user files to surprising locations. At plan time, both packages reported `0.0%` coverage despite being on critical filesystem paths.

## Current state

Relevant files:

- `internal/infra/config/loader.go` — loads TOML config and resolves `appimage_dir`.
- `internal/infra/xdg/dirs.go` — resolves XDG base directories and derived paths.

Current config behavior:

```go
// internal/infra/config/loader.go:30-56
func Load(path string, dirs xdg.Dirs) (app.Config, error) {
    cfg := DefaultAppConfig(dirs)
    bytes, err := os.ReadFile(path)
    if errors.Is(err, os.ErrNotExist) { return cfg, nil }
    // parse TOML
    if fileCfg.AppImageDir != "" { cfg.AppImageDir = resolveUserPath(fileCfg.AppImageDir) }
    return cfg, nil
}
```

```go
// internal/infra/config/loader.go:59-83
func resolveUserPath(path string) (string, error) {
    path = strings.TrimSpace(path)
    if path == "~" { return os.UserHomeDir() }
    if strings.HasPrefix(path, "~/") { return filepath.Join(home, strings.TrimPrefix(path, "~/")), nil }
    return filepath.Clean(path), nil
}
```

Current XDG behavior:

```go
// internal/infra/xdg/dirs.go:18-29
func Resolve() (Dirs, error) {
    home, err := os.UserHomeDir()
    return Dirs{
        ConfigHome: envOrDefault("XDG_CONFIG_HOME", filepath.Join(home, ".config")),
        DataHome: envOrDefault("XDG_DATA_HOME", filepath.Join(home, ".local", "share")),
        CacheHome: envOrDefault("XDG_CACHE_HOME", filepath.Join(home, ".cache")),
        StateHome: envOrDefault("XDG_STATE_HOME", filepath.Join(home, ".local", "state")),
    }, nil
}
```

## Commands you will need

| Purpose | Command | Expected on success |
|---|---|---|
| Config tests | `go test ./internal/infra/config` | exit 0 |
| XDG tests | `go test ./internal/infra/xdg` | exit 0 |
| Coverage check | `go test ./internal/infra/config ./internal/infra/xdg -cover` | both packages > 0% |
| Full tests | `go test ./...` | exit 0 |
| Full gate | `make verify` | exit 0 |

## Scope

**In scope**:

- Create `internal/infra/config/loader_test.go`
- Create `internal/infra/xdg/dirs_test.go`
- `plans/README.md` status update only

**Out of scope**:

- Changing config file format.
- Changing XDG defaults unless tests expose an obvious bug and the operator approves.
- Adding dependencies.

## Steps

### Step 1: Add XDG path tests

Create `internal/infra/xdg/dirs_test.go`. Cover:

- `Resolve` uses `$HOME` defaults when XDG env vars are unset.
- `Resolve` honors `XDG_CONFIG_HOME`, `XDG_DATA_HOME`, `XDG_CACHE_HOME`, `XDG_STATE_HOME`.
- `ConfigFile`, `DataDir`, `DefaultAppImageDir`, `DesktopDir`, `IconDir`, `CacheDir`, and `StateDir` join paths as expected for a manually constructed `Dirs` value.

Use `t.Setenv`. Avoid relying on the developer's real home; set `HOME` in tests.

**Verify**: `go test ./internal/infra/xdg` → exit 0.

### Step 2: Add config default and missing-file tests

Create `internal/infra/config/loader_test.go`. Cover:

- `DefaultAppConfig` maps XDG dirs to expected config/appimage/cache/desktop/icon paths.
- `Load` returns defaults when config file does not exist.
- Empty config file returns defaults.

Use `t.TempDir` and explicit `xdg.Dirs` values.

**Verify**: `go test ./internal/infra/config` → exit 0.

### Step 3: Add config parsing and path-resolution tests

In `loader_test.go`, cover:

- Valid TOML with `appimage_dir = "/custom/appimages"` overrides only `AppImageDir`.
- `appimage_dir = "~/Apps"` expands to `$HOME/Apps`.
- `appimage_dir = "~"` expands to `$HOME`.
- Whitespace is trimmed and non-home paths are cleaned.
- Malformed TOML returns an error containing `parse config file`.

Use `t.Setenv("HOME", tempHome)` for home expansion.

**Verify**: `go test ./internal/infra/config` → exit 0.

### Step 4: Run coverage and full verification

**Verify**:

- `go test ./internal/infra/config ./internal/infra/xdg -cover` → both packages > 0%.
- `go test ./...` → exit 0.
- `go vet ./...` → exit 0.
- `make verify` → exit 0.

## Done criteria

- [ ] `internal/infra/config` has tests for defaults, missing config, valid TOML, malformed TOML, and `~` expansion.
- [ ] `internal/infra/xdg` has tests for env overrides and derived helper paths.
- [ ] Both packages report > 0% coverage.
- [ ] `go test ./...`, `go vet ./...`, and `make verify` pass.
- [ ] No behavior changes unless explicitly justified by a failing test and approved.
- [ ] `plans/README.md` status row updated.

## STOP conditions

Stop and report if:

- Tests reveal behavior that contradicts README/user expectations and changing it would alter user file locations.
- `os.UserHomeDir` behaves unexpectedly in the test environment even with `HOME` set.
- A fix requires moving config/XDG logic across layers.

## Maintenance notes

When adding future config fields, extend `loader_test.go` with default, parse, and invalid-value coverage. Reviewers should ensure tests never read or write real user home paths.
