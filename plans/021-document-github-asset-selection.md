# Plan 021: Make GitHub asset selection discoverable

> **Executor instructions**: Follow this plan step by step. Run every verification command and confirm expected results. If a STOP condition occurs, stop and report. Update `plans/README.md` when done.
>
> **Drift check (run first)**: `git diff --stat 0cee363..HEAD -- README.md internal/cli/command/add/command.go internal/cli/command/update/command.go internal/app/github_asset_selection.go internal/app/github_asset_selection_test.go`

## Status

- **Priority**: P3
- **Effort**: S
- **Risk**: LOW
- **Depends on**: none
- **Category**: direction
- **Planned at**: commit `0cee363`, 2026-06-13

## Why this matters

Many GitHub releases publish multiple AppImages by architecture, flavor, or channel. `aim` already supports `--asset` patterns for add and update-source configuration, and ambiguity errors are intentional. README examples only show automatic selection, so users discover `--asset` after a failure instead of before.

## Current state

Evidence:

```go
// internal/cli/command/add/command.go:102-104
cmd.Flags().StringVar(&assetPattern, "asset", "", "match the GitHub AppImage asset name using filepath.Match syntax")
```

```go
// internal/cli/command/update/command.go:104-109
cmd.Flags().StringVar(&assetPattern, "asset", "", "match the GitHub AppImage asset name using filepath.Match syntax")
```

```md
<!-- README.md:35-39 -->
aim add ./Example.AppImage
aim add --github owner/repo
aim add --github owner/repo --prerelease
```

```md
<!-- README.md:50-54 -->
aim update --set example-app --github owner/repo
aim update --set example-app --github owner/repo --prerelease
aim update --set example-app --embedded
```

Ambiguity is tested in `internal/app/github_asset_selection_test.go`.

## Commands you will need

| Purpose | Command | Expected on success |
|---|---|---|
| App asset tests | `go test ./internal/app` | exit 0 |
| Add command tests | `go test ./internal/cli/command/add` | exit 0 |
| Update command tests | `go test ./internal/cli/command/update` | exit 0 |
| Full tests | `go test ./...` | exit 0 |
| Full gate | `make verify` | exit 0 |

## Scope

**In scope**:

- `README.md`
- Optional help text in `internal/cli/command/add/command.go`
- Optional help text in `internal/cli/command/update/command.go`
- Command tests only if help/validation changes
- `plans/README.md` status update only

**Out of scope**:

- Adding `--list-assets` or preview behavior; that is a separate M-sized feature.
- Changing asset scoring/selection semantics.
- Changing `filepath.Match` pattern rules.

## Steps

### Step 1: Add README examples for `--asset`

Update README add examples to include an asset pattern, for example:

```sh
aim add --github owner/repo --asset '*x86_64.AppImage'
```

Update update-source examples similarly:

```sh
aim update --set example-app --github owner/repo --asset '*x86_64.AppImage'
```

Add one sentence explaining `--asset` uses Go `filepath.Match` syntax and is useful when a release has multiple AppImage assets.

**Verify**: no command; README diff is clear.

### Step 2: Improve flag help only if needed

Read current flag help. If it already says "filepath.Match syntax" clearly, do not change code. If you improve wording, update command tests only if they assert help text.

**Verify**: `go test ./internal/cli/command/add ./internal/cli/command/update` → exit 0.

### Step 3: Confirm existing asset behavior tests still pass

Run app and command tests to ensure docs/help-only changes did not accidentally touch behavior.

**Verify**:

- `go test ./internal/app` → exit 0.
- `go test ./internal/cli/command/add ./internal/cli/command/update` → exit 0.

### Step 4: Run full verification

**Verify**:

- `go test ./...` → exit 0.
- `make verify` → exit 0.

## Done criteria

- [ ] README documents `--asset` for both GitHub add and GitHub update-source workflows.
- [ ] README mentions `filepath.Match`-style patterns and why users need them.
- [ ] Asset selection behavior is unchanged.
- [ ] `go test ./...` and `make verify` pass.
- [ ] `plans/README.md` status row updated.

## STOP conditions

Stop and report if:

- The desired user experience is asset preview/listing rather than docs-only discoverability.
- Existing help/docs already contain equivalent examples after drift.
- Updating help text breaks command tests unexpectedly.

## Maintenance notes

If users still struggle after docs improvements, consider a follow-up feature: `aim add --github owner/repo --list-assets` or asset candidates in `update --check` output.
