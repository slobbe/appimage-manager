# Layer Cleanup Checklist

This checklist tracks the remaining cleanup after the app-service DTO migration and legacy bridge removal.

## Current Status

Completed:

- [x] Removed app-service legacy bridge fields such as `LegacyApp`, `LegacySource`, and `LegacyEmbeddedUpdate`.
- [x] Removed `ListResult.ManagedApps`.
- [x] Removed `UpdateApplyBatchResult.LegacyResults`.
- [x] Moved applied-update persistence and superseded-app cleanup behind `internal/app/services.SourceUpdateService`.
- [x] Removed direct `internal/app/clock` and `internal/app/integrate` imports from `internal/cli/update_workflow.go`.
- [x] Removed `internal/domain` and `internal/app/update` imports from `internal/cli/commands.go`.
- [x] Removed `internal/domain` imports from `internal/cli/output.go`.
- [x] Removed `internal/domain` and `internal/app/discovery` imports from `internal/cli/package_sources.go`.
- [x] Removed `internal/domain` and `internal/app/update` imports from `internal/cli/update_workflow.go`.
- [x] Tightened the architecture allowlist for removed seams.

Remaining allowlisted production CLI files:

- None.

The goal is to make regular CLI files depend only on `internal/app/services` plus presentation/framework packages. Runtime composition files such as `runtime.go` and `runtime_wiring.go` may continue wiring concrete app/infra/domain dependencies.

## Target State

- [x] Normal CLI command/workflow files do not import `internal/domain`.
- [x] Normal CLI command/workflow files do not import lower-level `internal/app/*` packages except `internal/app/services`.
- [x] CLI constructs domain-free request DTOs and receives domain-free result DTOs.
- [x] App services own workflow coordination, persistence, cache updates, and domain conversion.
- [x] CLI owns only argument parsing, prompts, progress display, output rendering, and user-facing error text.
- [x] `scripts/check-architecture.sh` has no migration allowlist for production CLI files, except true runtime/composition exceptions.

## Cleanup Plan

### 1. Remove `internal/domain` from `internal/cli/commands.go`

Current reasons:

- Add/install flows still pass `domain.PackageRef` and `domain.PackageMetadata` through CLI helpers.
- Update set/unset still builds `domain.UpdateSource` in CLI.
- Some version/source rendering uses domain constants and helpers.
- Add integration overwrite prompt still receives domain update sources.

Tasks:

- [x] Introduce/finish domain-free app-service input DTOs for package refs.
  - Candidate types already present/nearby: `ProviderRef`, `PackageView`.
  - Add request DTOs if needed, e.g. `PackageRefInput` or use `ProviderRef` consistently.
- [x] Move `resolveUpdateSourceFromSetFlags` output from `*domain.UpdateSource` to an app-service input DTO.
- [x] Add app-service conversion/validation for update source input.
- [x] Change `UpdateSourceRequest` to accept domain-free source input or add a separate `SetSourceInputRequest`.
- [x] Move update-source equality/summary helpers that need domain constants behind app-service view helpers or use string-only constants in CLI.
- [x] Replace `appupdate.ManagedUpdateDownloadFilename` usage with an app-service wrapper/helper if still needed from CLI.
- [x] Remove `internal/domain` and `internal/app/update` imports from `internal/cli/commands.go`.
- [x] Remove the `commands.go` allowlist entry from `scripts/check-architecture.sh`.

Validation:

- [x] `go test ./internal/cli`
- [x] `go test ./internal/app/services`
- [x] `make test-architecture`

### 2. Remove `internal/domain` from `internal/cli/output.go`

Current reasons:

- `newUpdateOutputRow` still accepts domain/app-update types via `models.App` and `pendingManagedUpdate`.
- `packageMetadataOutput` still renders `domain.PackageMetadata` for install/dry-run flows.

Tasks:

- [x] Change update row construction to accept `appservices.AppSummary` / `AppDetails` and `ManagedUpdateView`.
- [x] Move any remaining domain package metadata JSON rendering to `PackageView`.
- [x] Replace `packageMetadataOutput` with a `PackageView`-based output helper or remove it if callers are migrated.
- [x] Remove `internal/domain` import from `internal/cli/output.go`.
- [x] Remove the `output.go` allowlist entry from `scripts/check-architecture.sh`.

Validation:

- [x] `go test ./internal/cli`
- [x] `make test-architecture`

### 3. Remove `internal/domain` and `internal/app/discovery` from `internal/cli/package_sources.go`

Current reasons:

- CLI parses package refs using `internal/app/discovery` directly.
- CLI still resolves metadata through discovery backends and returns `domain.PackageMetadata` for some install/dry-run paths.
- Domain and view package helper pairs still coexist.

Tasks:

- [x] Move package ref parsing into app services, or expose it through `internal/app/services`.
- [x] Move package metadata resolution for add/info/dry-run into `DiscoveryService` / `AddService` calls.
- [x] Replace `resolvePackageMetadataFromRef` and `resolvePackageMetadataFromInput` with app-service methods returning `PackageView` or install-ready app-service DTOs.
- [x] Convert ambiguity resolution to operate on `PackageView` or app-service input/output only.
- [x] Remove remaining domain-based package helper functions.
- [x] Remove `internal/domain` and `internal/app/discovery` imports from `internal/cli/package_sources.go`.
- [x] Remove the `package_sources.go` allowlist entry from `scripts/check-architecture.sh`.

Validation:

- [x] `go test ./internal/cli`
- [x] `go test ./internal/app/services`
- [x] `make test-architecture`

### 4. Remove `internal/domain` and `internal/app/update` from `internal/cli/update_workflow.go`

Current reasons:

- CLI still owns managed update check orchestration.
- CLI still manipulates `appupdate.ManagedUpdate`, check cache files, and domain apps.
- CLI still handles metadata update persistence after checks.
- Progress rendering uses `appupdate.ManagedApplyEvent` aliases.

Tasks:

- [x] Add an app-service check workflow that returns domain-free update rows/statuses and pending update handles/views.
- [x] Move update-check cache read/write/invalidation coordination behind app/runtime ports.
- [x] Move check metadata persistence behind app-service coordination.
- [x] Convert `managedUpdateRunConfig` to hold app-service requests/results instead of domain apps and app-update values.
- [x] Convert `managedUpdateCollection.pending` away from `[]appupdate.ManagedUpdate` in CLI.
- [x] Convert progress event/reporting types to app-service DTOs or callbacks so CLI does not import `internal/app/update`.
- [x] Remove `pendingManagedUpdate`, `managedCheckResult`, and app-update aliases from CLI.
- [x] Remove `internal/domain` and `internal/app/update` imports from `internal/cli/update_workflow.go`.
- [x] Remove the `update_workflow.go` allowlist entry from `scripts/check-architecture.sh`.

Validation:

- [x] `go test ./internal/app/services`
- [x] `go test ./internal/app/update`
- [x] `go test ./internal/cli`
- [x] `make test-architecture`
- [x] `make check`

### 5. Migrate legacy CLI tests and shrink test allowlist

Current reason:

- `internal/cli/commands_integration_test.go` remains a broad integration-style test file and is still allowlisted for direct domain/app/infra imports.

Tasks:

- [x] Split app-service-bound command tests into focused tests using fake `internal/app/services`.
- [x] Keep true integration/wiring tests separate and explicit.
- [x] Reduce `commands_test.go` imports by moving domain/app/infra setup to app/service or runtime-wiring tests where appropriate.
- [x] Remove or narrow the `commands_test.go` test allowlist in `scripts/check-architecture.sh`.

Validation:

- [x] `go test ./internal/cli`
- [x] `make test-architecture`

## Final Acceptance Criteria

- [x] `grep` for normal production CLI imports shows only `internal/app/services` among internal layers, excluding `runtime.go` and `runtime_wiring.go`.
- [x] `scripts/check-architecture.sh` has no production CLI migration allowlist entries.
- [x] No app-service result types contain hidden domain legacy bridge fields.
- [x] `make test-architecture` passes.
- [x] `make check` passes.

Useful commands:

```sh
grep -R '"github.com/slobbe/appimage-manager/internal/\(app\|domain\|infra\)' internal/cli/*.go
make test-architecture
make check
```
