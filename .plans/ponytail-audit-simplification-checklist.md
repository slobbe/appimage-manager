# Ponytail audit simplification checklist

Scope: over-engineering and complexity cuts only. Apply as focused commits. After each commit-sized item is implemented, the agent must validate the change and check off that item in this file.

Suggested validation baseline after each item:

- `go test ./...`
- If dependencies changed: `go mod tidy` and re-run `go test ./...`

## Commit 1: remove zero-value constructors

- [x] Replace constructors that only return `Type{}` with direct zero-value construction.
  - Candidates:
    - `appimage.NewExtractor()` -> `appimage.Extractor{}`
    - `desktop.NewDiscoverer()` -> `desktop.Discoverer{}`
    - `icon.NewDiscoverer()` -> `icon.Discoverer{}`
    - `download.NewDownloader()` -> `download.Downloader{}`
    - `fileutil.NewRemover()` -> `fileutil.Remover{}` or later commit’s replacement
    - `selfupdate.NewInstaller()` -> `selfupdate.Installer{}`
  - Remove the now-unused constructor functions.
  - Update tests that call those constructors.
  - Validate with `go test ./...`.
  - Check this item when validation passes.

## Commit 2: shrink artifact removal abstraction

- [x] Simplify `ArtifactRemover`, which currently wraps `os.Remove` behind a one-method object.
  - Prefer a function-shaped port or a small `fileutil.RemoveArtifact(ctx, path)` function wired from `cmd/aim`.
  - Keep app-layer dependency direction intact: `internal/app` must not import `internal/infra`.
  - Preserve current behavior: validate non-empty path, respect canceled context, ignore missing files.
  - Remove `internal/infra/fileutil/remover.go` if replaced by a function in a more appropriate file.
  - Update service tests/fakes.
  - Validate with `go test ./...`.
  - Check this item when validation passes.

## Commit 3: deduplicate time formatting helpers

- [x] Remove duplicated `formatSourceTime` helpers.
  - Current duplicates exist in:
    - `internal/cli/command/info/command.go`
    - `internal/cli/output/info.go`
    - `internal/infra/storage/repository.go`
  - Consolidate only where it does not violate layer rules. Do not make domain know about JSON/storage formatting.
  - For CLI text and JSON output, prefer one helper in `internal/cli/output` or inline the small `UTC().Format(time.RFC3339)` logic.
  - Validate with `go test ./...`.
  - Check this item when validation passes.

## Commit 4: delete unused build metadata

- [ ] Delete empty/unused build metadata leftovers.
  - Remove empty `cmd/aim/buildinfo.go`.
  - Remove unused `commit` and `date` variables from `cmd/aim/main.go` unless they are surfaced in output first.
  - Validate with `go test ./...`.
  - Check this item when validation passes.

## Commit 5: reduce command test scaffolding

- [ ] Replace broad `commandtest.Service` no-op implementation with smaller test fakes.
  - Current file: `internal/cli/command/commandtest/service.go`.
  - Each command test usually needs only one service method; prefer local minimal fakes or another smaller pattern.
  - Remove `commandtest.Service` once no tests embed it.
  - Validate with `go test ./...`.
  - Check this item when validation passes.

## Done criteria

- [ ] All selected checklist items are checked off only after their implementation commit validates.
- [ ] Final validation: `go test ./...` passes.
- [ ] If dependencies changed, `go.mod` and `go.sum` are tidy.
