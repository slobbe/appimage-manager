# Layered Architecture Decisions

This document records the agreed target architecture for AppImage Manager as a reference point for ongoing refactoring.

## Agreed decisions

1. **CLI must be thin**
   - Parse flags and arguments.
   - Prompt, render output, show progress, and handle terminal behavior.
   - Call application services.
   - Map errors to user-facing output and exit codes.
   - Do not contain business workflow orchestration.

2. **CLI talks to command-shaped app service facades**
   - Normal CLI command code should depend on `internal/app/services`.
   - Normal CLI command code should not call lower app packages such as `internal/app/update`, `internal/app/integrate`, `internal/app/remove`, or similar workflow-building packages directly.

3. **CLI should not import `internal/domain`**
   - Domain entities and value objects should not leak into normal CLI command code.
   - The application service boundary should provide CLI-facing request/result/view types.

4. **App service DTOs should be domain-free**
   - `internal/app/services` should expose request, result, and view DTOs such as:
     - `AppSummary`
     - `AppDetails`
     - `UpdateSourceView`
     - `PackageView`
     - `AppImageInfoView`
     - typed `DryRunPlan`
   - App services map from domain/internal app types to these boundary DTOs.

5. **Runtime wiring can stay in `internal/cli`**
   - `internal/cli/runtime.go` and `internal/cli/runtime_wiring.go` are explicit composition exceptions.
   - They may import infrastructure, domain, and lower app packages for production wiring only.

6. **Normal CLI tests should use fake `app/services`**
   - Long term, normal CLI command tests should avoid importing domain, infrastructure, or lower app packages.
   - Wiring or end-to-end tests may use real adapters, but should be clearly scoped as exceptions.

7. **Callbacks are allowed, but domain-free**
   - App services may accept confirmation, ambiguity-resolution, and progress/reporting callback interfaces.
   - CLI owns terminal interaction and prompt text.
   - Callback request/event types should live in `internal/app/services` and should not expose domain or lower app types.

8. **CLI owns final output format**
   - App services expose semantic domain-free DTOs.
   - CLI owns command-specific JSON, CSV, and text envelopes/rendering.
   - Avoid `map[string]interface{}` plans containing domain objects.

9. **Request/input DTOs should also be domain-free**
   - Use app-service types such as `ProviderRef` and `UpdateSourceInput` instead of `domain.PackageRef` and `domain.UpdateSource` at the CLI boundary.
   - App services convert these inputs internally to domain types.

10. **App services expose semantic error kinds**
    - Use a small taxonomy for CLI mapping, for example:
      - invalid input
      - not found
      - unavailable
      - permission
      - conflict
      - canceled
      - internal
    - CLI maps these app-service errors to presentation-specific errors and exit behavior.

11. **Architecture enforcement should be gradual**
    - `scripts/check-architecture.sh` should prevent new normal CLI boundary violations.
    - Existing violations may be temporarily allowlisted and removed as commands are migrated.

12. **Migration order**
    1. Add architecture check scaffolding with a temporary allowlist.
    2. Add app-service DTO foundation.
    3. Add app-service error-kind contract.
    4. Migrate `list`.
    5. Migrate `remove`.
    6. Migrate `info`.
    7. Migrate `add`.
    8. Migrate `update set/unset`.
    9. Migrate update check/apply workflow.
    10. Migrate normal CLI tests to fake services.
    11. Tighten and eventually remove the temporary allowlist.

## Target dependency shape

```txt
cmd/aim
  -> internal/cli

internal/cli normal command files
  -> internal/app/services

internal/cli/runtime.go and internal/cli/runtime_wiring.go
  -> internal/app/services
  -> lower internal/app packages
  -> internal/domain
  -> internal/infra

internal/app/services
  -> lower internal/app packages
  -> internal/domain

internal/app
  -> internal/domain

internal/infra
  -> internal/app
  -> internal/domain
```

## Boundary rule for normal CLI files

Normal files in `internal/cli` should not import:

- `internal/domain`
- `internal/infra`
- lower `internal/app/*` packages other than `internal/app/services`

Explicit production composition exceptions:

- `internal/cli/runtime.go`
- `internal/cli/runtime_wiring.go`

Temporary migration exceptions are tracked in `scripts/check-architecture.sh` and should shrink over time.
