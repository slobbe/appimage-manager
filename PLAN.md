# Layer Boundary Cleanup Plan

This checklist is based on a strict but pragmatic layer-boundary review of the current codebase. The repo already has a useful target shape with `internal/domain`, `internal/app`, `internal/infra`, and `internal/cli`, plus `internal/architecture/import_boundaries_test.go` enforcing import direction. The remaining coupling is mostly behavioral: CLI still owns workflows and runtime wiring, app packages still contain IO/API/process details, and some pure business rules live above the domain layer.

Target dependency direction:

```txt
cmd/aim
  -> internal/cli
  -> internal/app
  -> internal/domain

internal/infra implements app/domain boundary interfaces and is wired at the edge.
```

## Findings

| Package / Folder | Observed Role | Expected Layer | Assessment |
|---|---|---|---|
| `cmd/aim` | Entrypoint | CLI | Good, already thin. |
| `internal/cli` | Cobra commands, output rendering, prompts, config, runtime wiring, lock files, downloader helpers, desktop validation shelling | CLI plus mixed composition/infra | Too much workflow and concrete IO lives here. |
| `internal/cli/config` | XDG path resolution, env reads, global mutable paths, settings file parsing | Infra/config | Should move out of CLI so app services receive explicit path/config values. |
| `internal/app/appimage` | AppImage inspection/extraction workflow plus path construction and desktop file reads through ports | App plus some domain/path rules | Keep orchestration, move pure validation/naming to domain and extraction/file details to infra. |
| `internal/app/discovery` | Provider metadata workflow, GitHub backend facade, URL construction | App plus provider-specific rules | Keep discovery use case; move provider-specific GitHub URL/API behavior to infra and pure package-ref rules to domain. |
| `internal/app/integrate` | Full integration workflow, identity resolution, path layout, icon install, desktop validation, persistence, cache refresh | App with domain and infra concerns mixed in | Highest-value use case to split after CLI is thinner. |
| `internal/app/update` | Update check/apply workflow, zsync metadata parsing, ELF `.upd_info` reading, HTTP client globals, staged download naming | App plus infra and domain rules | Move ELF/HTTP/zsync/staging to infra; move release/update selection rules to domain. |
| `internal/app/upgrade` | Self-upgrade workflow, GitHub/raw URL construction, HTTP client globals, installer execution through port | App plus infra details | Should depend on `ReleaseFinder` and `SelfUpdater` ports only. |
| `internal/app/remove` | Remove workflow and path cleanup | App | Mostly acceptable, but filesystem operations should stay behind explicit ports. |
| `internal/domain` | App, source, update, identity, slug, version, AppImage desktop parsing | Domain | Mostly pure and useful. Some more business rules can move here from app/CLI. |
| `internal/infra/*` | Filesystem, repository, download, GitHub, desktop, zsync, selfupdate, appimage extraction | Infra | Good package split. Some release-selection behavior may belong in domain/app if it is a business decision rather than API adaptation. |

## Refactoring Checklist

### 1. Thin the CLI first: CLI should parse flags and call app services

- [x] Add a thin app-service API for each command path before moving logic:
  - `AddService`
  - `ListService`
  - `InfoService`
  - `RemoveService`
  - `UpdateService`
  - `UpgradeService`
  - `DiscoveryService`
- [ ] Move command workflow decisions out of `internal/cli/commands.go`; keep only flag parsing, superficial argument validation, prompts, progress display, output formatting, and exit/error rendering.
- [ ] Convert `runIntegrateTarget`, install target handling, managed update apply, recovery, and dry-run planning into app service calls that return structured results.
- [x] Split `internal/cli/app_ports.go` into a composition root, for example `internal/cli/runtime` or `internal/bootstrap`, whose job is only to construct app services with infra adapters.
- [x] Move lock-file handling from `internal/cli/state_lock.go` behind an app-level `StateLock` or runtime-level `Locker` so commands do not directly own persistence coordination.
- [x] Move `internal/cli/config` out of CLI, likely to `internal/infra/config`, and pass resolved paths/settings into app services explicitly.
- [x] Keep CLI output helpers such as JSON rendering, progress UI, prompts, and friendly error text in `internal/cli`.
- [x] Add command tests that assert each command calls the expected service with parsed inputs, using service fakes instead of real repository/download/filesystem setup.
- [ ] Suggested commits:
  - `refactor(cli): introduce command service interfaces`
  - `refactor(cli): move runtime wiring out of commands`
  - `refactor(config): move xdg config loading to infra`

### 2. Extract pure domain types/functions: Move business concepts and rules into domain

- [x] Move managed app identity decisions from `internal/app/integrate/managed_identity.go` into `internal/domain`, keeping repository lookup orchestration in app.
- [x] Move app ID, desktop stem, slug, replacement, and stale-icon ownership rules into pure domain functions that accept values and return decisions.
- [x] Move update availability, release transport selection, version comparison, and update source validation into domain where they are pure business decisions.
- [x] Move pure package reference parsing/normalization from `internal/app/discovery` into domain if it describes the package model rather than a provider API.
- [x] Keep `ParseDesktopEntryAppInfo` pure in domain, but keep actual desktop file reading/rewriting in infra.
- [x] Keep domain free of `os`, `net/http`, `debug/elf`, filesystem APIs, env vars, Cobra, concrete infra packages, and JSON/TOML DTOs.
- [x] Add focused domain tests for identity collision, replacement decisions, release/update availability, package refs, and path-independent AppImage metadata parsing.
- [ ] Suggested commits:
  - `refactor(domain): extract managed app identity rules`
  - `refactor(domain): extract update selection rules`
  - `test(domain): cover pure package and update decisions`

### 3. Create app services: Move workflows into app

- [ ] Replace package-level functions and global setters in app packages with explicit service structs and constructors, for example `integrate.Service`, `update.Service`, `upgrade.Service`, and `discovery.Service`.
- [ ] Move full workflows currently embedded in CLI into app services:
  - add local AppImage
  - add managed package ref
  - check updates
  - apply updates
  - remove app
  - self-upgrade
  - dry-run planning
- [ ] Make app services depend on domain plus small ports, not concrete `infra` packages and not CLI callbacks except narrow user-decision ports such as `UpdateOverwriteConfirmer`.
- [x] Return structured result types from app services so CLI can render text/JSON without app knowing about terminal formatting.
- [ ] Remove global mutable defaults like `SetFilesystem`, `SetGitHubReleaseResolver`, `SetSelfUpdater`, and `SharedHTTPClient` from app packages after services are wired explicitly.
- [ ] Keep transaction/workflow coordination in app: load app, decide with domain, call repository/downloader/filesystem ports, persist result, refresh caches.
- [x] Add app tests with fakes for repositories, downloaders, release finders, filesystem, AppImage extractor, desktop integration, clock, and locker.
- [ ] Suggested commits:
  - `refactor(app): introduce integrate service`
  - `refactor(app): introduce update service`
  - `refactor(app): introduce upgrade service`
  - `refactor(app): remove global app ports`

### 4. Move IO/API/filesystem code into infra: Storage, HTTP, GitHub, config, desktop files, etc.

- [x] Move ELF `.upd_info` extraction from `internal/app/update/check.go` into an infra AppImage/update-info reader.
- [x] Move zsync metadata fetching and parsing that is tied to wire format into `internal/infra/zsync`; return app/domain-level metadata structs.
- [x] Move staged download naming, HTTP metadata, progress adaptation, status errors, and shared HTTP client ownership out of CLI/app into `internal/infra/download`.
- [x] Keep GitHub API DTOs, release asset HTTP lookup, repository metadata lookup, and GitHub URL construction in `internal/infra/github`.
- [x] Move self-update HTTP calls, installed binary resolution, installer script execution, and raw GitHub install-script URL handling to `internal/infra/selfupdate`.
- [x] Keep desktop entry rewrite, validation command execution, link resolution, cache refresh, and icon filesystem behavior in `internal/infra/desktop` or `internal/infra/filesystem`.
- [x] Keep repository JSON persistence and validation in `internal/infra/repository`; app should see only repository/store interfaces.
- [x] Move XDG/env/settings loading and directory creation to `internal/infra/config`; pass a plain `Paths`/`Settings` value into app services.
- [x] Keep infra tests integration-oriented around filesystem, HTTP test servers, command execution fakes, repository persistence, and DTO mapping.
- [ ] Suggested commits:
  - `refactor(update): move upd info reader to infra`
  - `refactor(download): centralize staged download IO`
  - `refactor(selfupdate): move installer IO to infra`
  - `refactor(config): isolate xdg path loading`

### 5. Add interfaces only at boundaries: Repository, Downloader, ReleaseFinder, etc. Avoid abstracting everything.

- [x] Define ports in the app package that owns the use case, only when crossing to infra or user interaction.
- [ ] Keep these likely ports:
  - `AppRepository` or `AppStore`
  - `PackageRepository` if package discovery/install grows beyond GitHub
  - `Downloader`
  - `StagedDownloader`
  - `ReleaseFinder`
  - `RepositoryMetadataFinder`
  - `UpdateInfoReader`
  - `ZsyncRunner`
  - `HashVerifier`
  - `AppImageExtractor`
  - `DesktopIntegrator`
  - `ConfigLoader`
  - `StateLocker`
  - `Clock`
- [x] Do not create interfaces for pure domain helpers, value constructors, formatting functions, or one-line wrappers that do not cross a real boundary.
- [x] Keep interfaces small and use-case-shaped; prefer `FindRelease(ctx, source)` over exposing an entire GitHub client.
- [x] Keep concrete infra adapters in `internal/infra/*`, with mapping code at the adapter boundary so DTOs do not leak into app/domain.
- [x] Extend `internal/architecture/import_boundaries_test.go` as cleanup progresses:
  - `internal/domain` must not import `internal/app`, `internal/infra`, or `internal/cli`.
  - `internal/app` must not import `internal/infra` or `internal/cli`.
  - `internal/infra` must not import `internal/cli`.
  - `internal/cli` may import app and bootstrap/runtime wiring, but command files should not import concrete infra once composition is isolated.
- [x] Run `go test ./... && go vet ./...` after each phase and before merging the final cleanup.
- [ ] Suggested commits:
  - `refactor(app): narrow boundary ports`
  - `test(architecture): enforce cli and infra boundaries`

## Final Target Structure

```txt
cmd/aim/

internal/domain/
  app.go
  identity.go
  package.go
  source.go
  update.go
  version.go

internal/app/
  add/
  appimage/
  discovery/
  integrate/
  remove/
  update/
  upgrade/

internal/infra/
  appimage/
  config/
  desktop/
  download/
  filesystem/
  github/
  repository/
  selfupdate/
  zsync/

internal/cli/
  commands.go
  output.go
  prompts.go
  progress.go
  errors.go
  runtime/

internal/architecture/
```

## Definition of Done

- [ ] CLI command handlers are thin and do not directly call repositories, HTTP clients, shell commands, desktop validators, or filesystem persistence except through service results/rendering.
- [x] Domain contains the stable business rules and has no IO, env, HTTP, CLI, or concrete infra imports.
- [ ] App services coordinate workflows with explicit dependencies and no package-level global setters.
- [x] Infra owns concrete filesystem, HTTP, GitHub, config, desktop, repository, self-update, AppImage extraction, and zsync behavior.
- [x] Interfaces exist only where app/domain cross into infra or user interaction.
- [x] `go test ./... && go vet ./...` passes.
