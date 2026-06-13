# Plan 002: Bound and verify GitHub asset downloads

> **Executor instructions**: Follow this plan step by step. Run every verification command and confirm the expected result before moving to the next step. If anything in the "STOP conditions" section occurs, stop and report — do not improvise. When done, update the status row for this plan in `plans/README.md`.
>
> **Drift check (run first)**: `git diff --stat 57f5ab0..HEAD -- internal/app/service.go internal/app/service_ports.go internal/infra/download/downloader.go internal/infra/download/downloader_test.go`
> If any in-scope file changed since this plan was written, compare the "Current state" excerpts against the live code before proceeding; on a mismatch, treat it as a STOP condition.

## Status

- **Priority**: P1
- **Effort**: S/M
- **Risk**: LOW-MED
- **Depends on**: none
- **Category**: security
- **Planned at**: commit `57f5ab0`, 2026-06-13

## Why this matters

GitHub release asset metadata includes expected byte size, and the app layer already passes that size into the downloader. The downloader currently streams the response body until EOF without enforcing the expected size or checking `Content-Length`. A compromised, inconsistent, or buggy asset endpoint can write more bytes than advertised, fill disk/cache, or write truncated content that later proceeds to AppImage extraction/integration.

## Current state

Relevant files:

- `internal/app/service.go` — passes GitHub release asset metadata to the downloader for add/update.
- `internal/app/service_ports.go` — defines `DownloadSource` and downloader port types.
- `internal/infra/download/downloader.go` — HTTP downloader implementation.
- `internal/infra/download/downloader_test.go` — focused HTTP downloader tests.

Current add/update call sites:

```go
// internal/app/service.go:177-181
 downloaded, err := s.downloads.Download(ctx, DownloadSource{
 	URL:       asset.DownloadURL,
 	FileName:  asset.Name,
 	SizeBytes: asset.SizeBytes,
 }, downloadPath, download)
```

```go
// internal/app/service.go:491-495
downloaded, err := s.downloads.Download(ctx, DownloadSource{
	URL:       plan.asset.DownloadURL,
	FileName:  plan.asset.Name,
	SizeBytes: plan.asset.SizeBytes,
}, downloadPath, download)
```

Current downloader behavior:

```go
// internal/infra/download/downloader.go:68-87
written, copyErr := copyWithProgress(ctx, destination, resp.Body, progress)
closeErr := destination.Close()
// ... error handling ...
if err := os.Rename(temporaryPath, destinationPath); err != nil {
	_ = os.Remove(temporaryPath)
	return app.DownloadedFile{}, fmt.Errorf("replace download %q: %w", destinationPath, err)
}

return app.DownloadedFile{Path: destinationPath, SizeBytes: written}, nil
```

```go
// internal/infra/download/downloader.go:90-118
func copyWithProgress(ctx context.Context, dst io.Writer, src io.Reader, progress app.DownloadProgress) (int64, error) {
	buffer := make([]byte, 32*1024)
	var written int64
	for {
		if err := ctx.Err(); err != nil {
			return written, err
		}
		// reads until EOF; no byte limit
	}
}
```

Current tests cover happy path, HTTP errors, invalid input, and cancellation, but not oversize/truncated responses.

Repo conventions to follow:

- Infra adapters return contextual errors with `fmt.Errorf("...: %w", err)` where applicable.
- Use `httptest.Server`, `t.TempDir`, and simple fake progress types as in `internal/infra/download/downloader_test.go`.
- Do not add dependencies.

## Commands you will need

| Purpose | Command | Expected on success |
|---|---|---|
| Focused downloader tests | `go test ./internal/infra/download` | exit 0, all tests pass |
| App tests | `go test ./internal/app` | exit 0, all tests pass |
| Full tests | `go test ./...` | exit 0, all packages pass |
| Vet | `go vet ./...` | exit 0, no diagnostics |
| Full local gate | `make verify` | exit 0 |

## Scope

**In scope**:

- `internal/infra/download/downloader.go`
- `internal/infra/download/downloader_test.go`
- `internal/app/service_ports.go` only if documentation/comments on `DownloadSource.SizeBytes` need clarification
- `plans/README.md` status update only

**Out of scope**:

- Adding checksums/signature verification.
- Changing GitHub API client behavior.
- Changing app update selection logic.
- Self-update install hardening; see `plans/004-avoid-mutable-script-self-update.md`.

## Git workflow

- Do not create commits unless explicitly asked.
- If committing later, use a message like `fix(download): verify release asset sizes`.

## Steps

### Step 1: Add failing downloader tests for size enforcement

In `internal/infra/download/downloader_test.go`, add tests for:

1. **Oversized response body**: `DownloadSource{URL: server.URL, SizeBytes: 5}` but server writes more than 5 bytes. Expect `Download` returns an error, removes `destination + ".tmp"`, and does not leave `destination`.
2. **Truncated response body**: expected size 20 but server writes fewer bytes. Expect error and no destination file.
3. **Mismatched `Content-Length` when present**: expected size 5, server sets `Content-Length: 10`. Prefer failing before writing any file.
4. **Unknown expected size remains supported**: `SizeBytes` 0 should keep current behavior for non-GitHub/unknown-size callers, unless you find all callers can require positive size. If you choose to reject zero, STOP and report because that is broader behavior.

Use `strings.Contains(err.Error(), "size")` or a similarly stable substring; do not assert the full error string.

**Verify**: `go test ./internal/infra/download` → new tests should fail before implementation.

### Step 2: Validate `Content-Length` against expected size

In `internal/infra/download/downloader.go`, after successful 2xx response and before creating the destination directory/file:

- If `source.SizeBytes > 0` and `resp.ContentLength >= 0` and `resp.ContentLength != source.SizeBytes`, return an error.
- Include the URL and both expected/actual sizes in the error message.
- Do not treat `ContentLength == -1` as an error; chunked responses can still be validated by bytes written.

**Verify**: `go test ./internal/infra/download` → Content-Length test should pass; body-size tests may still fail until Step 3.

### Step 3: Enforce a maximum while copying and exact size after copying

Still in `downloader.go`:

- If `source.SizeBytes > 0`, wrap `resp.Body` in a limiting reader that detects more than expected bytes. A simple approach: use `io.LimitReader(resp.Body, source.SizeBytes+1)` and after copy, if `written > source.SizeBytes`, return an error.
- After copy, if `source.SizeBytes > 0` and `written != source.SizeBytes`, remove the temporary file and return an error.
- Ensure progress advances only for bytes actually written.
- Preserve existing cancellation behavior.

Be careful: if you return a size error after `copyWithProgress`, do it before final `os.Rename`, and remove the temp file just like other write failures.

**Verify**: `go test ./internal/infra/download` → all downloader tests pass.

### Step 4: Clarify port documentation if useful

If `DownloadSource.SizeBytes` lacks a comment in `internal/app/service_ports.go`, add a concise app-layer comment explaining that values `> 0` are expected bytes and should be enforced by download adapters; `0` means unknown.

Do not change the interface shape unless absolutely necessary.

**Verify**: `go test ./internal/app ./internal/infra/download` → both pass.

### Step 5: Run full verification

**Verify**:

- `go test ./...` → all packages pass.
- `go vet ./...` → exit 0.
- `make verify` → exit 0.

## Test plan

- Add downloader tests for oversized, truncated, mismatched `Content-Length`, and unknown-size success.
- Existing tests in `internal/infra/download/downloader_test.go` remain the pattern.
- No network calls outside local `httptest.Server`.

## Done criteria

- [ ] `Downloader.Download` rejects positive expected-size downloads when `Content-Length` contradicts expected size.
- [ ] `Downloader.Download` rejects bodies larger or smaller than positive `DownloadSource.SizeBytes`.
- [ ] Failed size validation removes temp files and does not install/rename destination files.
- [ ] `SizeBytes == 0` keeps current unknown-size behavior unless the operator approved a breaking change.
- [ ] `go test ./internal/infra/download`, `go test ./...`, `go vet ./...`, and `make verify` pass.
- [ ] `plans/README.md` status row updated.

## STOP conditions

Stop and report if:

- The drift-check excerpts do not match live code.
- Enforcing positive sizes requires changing app service workflows outside download call sites.
- Tests reveal GitHub asset sizes are sometimes intentionally zero/unknown in existing app tests and the desired behavior is unclear.
- Any verification fails twice after focused fixes.

## Maintenance notes

This plan does not provide cryptographic integrity. It only ensures the bytes written match the release metadata already used by `aim`. If release checksums/signatures are added later, they should build on this size enforcement rather than replace it.