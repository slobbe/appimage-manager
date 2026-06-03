# Codebase shrink plan

This note captures the current architecture simplification plan for `aim`.

## Diagnosis

The codebase does not feel too large because it has layers. The layers are still useful:

```txt
cmd/aim
internal/cli
internal/app
internal/domain
internal/infra
```

It feels too large because several implementation patterns add indirection beyond what the feature set needs:

- `internal/cli` contains runtime/composition wiring in addition to command parsing and rendering.
- Runtime behavior is partly configured through package-level mutable hooks.
- Some app packages expose zero-value `Service{}` wrapper functions that bypass explicit dependency injection.
- App-level workflows are split between app packages and CLI runtime wiring.
- `internal/app/services` duplicates many domain/update/discovery structs as command-oriented views.
- Large facade files hide multiple use cases and make ownership harder to see.

## Target boundaries

```txt
cmd/aim
  minimal entrypoint, signal context, version injection

internal/cli
  command definitions, flags, prompts, progress display, output formatting, error rendering

internal/app
  use-case orchestration, domain decisions, concrete infra usage, structured results

internal/domain
  domain entities, value objects, business rules

internal/infra
  concrete filesystem/http/github/desktop/repository adapters used by app workflows
```

`internal/cli` may depend on app-service interfaces/results, but regular command files should not know about concrete infra adapters or workflow wiring. `internal/app` may import `internal/infra`; `internal/infra` should not import `internal/app`.

## PR 1: remove mutable runtime service wiring

Goal: make production behavior explicit and stop configuring runtime workflows by mutating package-level function hooks.

Current problems:

- `internal/cli/runtime_wiring.go` has globals such as `integrateLocalApp`, `integrateExistingApp`, `removeManagedApp`, `addAppsBatch`, and `addSingleApp`.
- `configureRepositoryStores()` mutates those globals after config has loaded.
- `sameFunc()` checks whether a function is still the default before replacing it.
- This makes production behavior depend on initialization order and makes tests rely on global mutation.

Desired direction:

- Move concrete `integrate.Service`, `remove.Service`, `appimage.Service`, and `update.Service` construction toward app-owned workflows.
- Let app workflows call concrete infra directly when that makes the design smaller.
- Avoid package-level mutation for production runtime behavior.

First incremental step:

- Remove the `configureRepositoryStores()` mutation phase.
- Replace zero-value wrapper defaults with explicit default service builders.
- Leave deeper test-hook cleanup for a follow-up if needed, but production wiring should no longer depend on `sameFunc()` mutation.

## PR 2: move workflow wiring out of CLI and into app

Goal: make `internal/cli` presentation-only.

Move from `internal/cli` into app-owned services/workflows:

- concrete adapter construction needed by use cases
- repository store construction for app workflows
- HTTP client and GitHub/downloader/zsync/self-update wiring
- update cache persistence
- staged download path helpers

Keep CLI-only concerns in CLI:

- flag parsing
- prompts and confirmations
- terminal progress display
- output formatting
- exit-code/error rendering

After this, CLI should call app services/use cases instead of constructing concrete dependencies itself.

## PR 3: delete zero-value service wrappers

Remove compatibility-style functions such as:

- `integrate.IntegrateFromLocalFile`
- `integrate.IntegrateFromLocalFileWithoutCacheRefresh`
- `integrate.IntegrateFromLocalFileWithoutCacheRefreshOrPersist`
- `integrate.IntegrateExisting`
- `integrate.MakeDesktopLink`
- `integrate.ValidateDesktopEntry`
- `integrate.InstallDesktopIcon`
- `integrate.ResolveManagedAppID`
- `integrate.FindEquivalentManagedApp`
- `remove.Remove`

Use explicitly constructed services instead.

## PR 4: reduce app-service DTO duplication

`internal/app/services/views.go` mirrors many domain and app/update types:

- `AppSummary`
- `AppDetails`
- `SourceView`
- `UpdateSourceView`
- `PackageView`
- `ManagedUpdateView`
- `ManagedApplyResultView`

Because the project is beta, internal result types do not need backwards compatibility. Prefer direct domain/app result types where practical, and keep CLI-only row structs in `internal/cli`.

## PR 5: split remaining giant files

After behavior and dependency simplification, split large files for readability:

- `internal/cli/commands.go`
- `internal/cli/output.go`
- `internal/cli/update_workflow.go`
- `internal/app/services/runtime.go`

Do this after deletion/refactoring so the split reflects the final ownership model instead of preserving current bloat in more files.
