# Layer Boundary Refactoring Plan

This plan preserves behavior, keeps commits small, makes each step independently testable, and prefers moving code over rewriting code.

## 1. Freeze behavior with a baseline

Suggested commit: `test: capture current architecture behavior`

- Run `go test ./... && go vet ./...`.
- Add or confirm focused tests around add/install, update check/apply, remove, upgrade, discovery, and repository persistence before moving more code.
- Use this baseline as the safety check for every later move.

## 2. Remove application-layer imports of concrete infrastructure

Highest-severity issue: `internal/app/...` imports `internal/infra/...` directly in packages like `appimage`, `discovery`, `integrate`, `remove`, `update`, and `upgrade`.

Suggested commit: `refactor(app): depend on ports instead of infra`

- Define small interfaces in the owning app packages for filesystem, desktop, download, zsync, GitHub release lookup, self-update, and AppImage extraction.
- Keep concrete implementations in `internal/infra/...`.
- Wire implementations from CLI/runtime.
- Test each use case package with fakes.

## 3. Move remaining IO and process execution out of application use cases

Suggested commit: `refactor(app): move os execution behind adapters`

- Remove direct `os`, `os/exec`, filesystem path mutation, and shell/process concerns from `internal/app/integrate`, `internal/app/update`, and `internal/app/upgrade`.
- Prefer moving existing code into infra adapters over rewriting.
- Keep use cases as orchestration only.
- Test with app-level fake ports plus existing integration-style infra tests.

## 4. Separate CLI runtime wiring from command behavior

Current issue: `internal/cli` imports app packages and infra packages together, so command code also acts as composition root.

Suggested commit: `refactor(cli): isolate runtime assembly`

- Move dependency construction into a narrow runtime/composition package under CLI or a dedicated bootstrap package.
- Commands should call app services through already-wired runtime dependencies.
- CLI tests should still assert command behavior and output, but not require concrete infra wiring except in runtime assembly tests.

## 5. Move CLI-owned formatting and prompts away from workflows

Suggested commit: `refactor(cli): isolate rendering and prompts`

- Keep Cobra, prompt text, progress UI, JSON rendering, and command output strictly under `internal/cli`.
- Ensure app packages return structured results/errors, not user-facing strings where avoidable.
- Test with existing command snapshot/output tests.

## 6. Keep domain pure and finish model consolidation

Current domain is mostly clean, but it imports parsing/formatting packages such as `net/url`, `path/filepath`, `regexp`, and string normalization helpers.

Suggested commit: `refactor(domain): consolidate pure domain models`

- Keep only stable business rules in `internal/domain`.
- Move provider-specific source parsing or filesystem-derived identity behavior out if it depends on external representation details.
- Preserve domain tests for versions, identity, packages, AppImage info, and slug behavior.

## 7. Make repository persistence private behind app stores

Suggested commit: `refactor(repository): hide persistence details behind stores`

- Keep `internal/infra/repository` as a concrete adapter.
- App packages should depend only on store interfaces already close to their use case packages.
- CLI should not call repository methods directly except in composition/runtime setup.
- Test repository package separately plus app package store-fake tests.

## 8. Split update concerns by source and transport

Suggested commit: `refactor(update): separate source selection from transport`

- Keep release selection and domain decisions separate from GitHub HTTP and zsync transport.
- Move GitHub API details to `internal/infra/github`.
- Move zsync execution/download staging to infra adapters.
- App update package should coordinate: inspect current app, resolve update candidate, apply selected update strategy.

## 9. Separate discovery metadata from GitHub implementation

Suggested commit: `refactor(discovery): isolate provider metadata`

- Keep app discovery interfaces and provider metadata rules in `internal/app/discovery`.
- Keep concrete GitHub calls and HTTP clients in infra.
- Test discovery with fake provider responses.

## 10. Add boundary guard tests

Suggested commit: `test: enforce layer import boundaries`

- Add a small architecture test that fails if:
  - `internal/domain` imports `internal/app`, `internal/infra`, or `internal/cli`
  - `internal/app` imports `internal/infra` or `internal/cli`
  - `internal/infra` imports `internal/cli`
- Run `go test ./... && go vet ./...`.
