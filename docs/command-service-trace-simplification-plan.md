# Command/Service Trace Simplification Plan

## Goal

Simplify each command/service trace so every command follows the same shape:

```txt
cobra command
  -> parse CLI flags/args into one request
  -> run one app-level use case
  -> render one structured result
```

Today several commands still have a split trace where CLI code chooses modes, calls multiple service methods, handles dry-run separately, manages locks/directories, and renders ad hoc output. The biggest examples are `add` and `update`, with smaller simplification opportunities in `remove`, `list`, `info`, and `self-update`.

Relevant current files:

- `internal/cli/commands.go`
- `internal/cli/update_workflow.go`
- `internal/cli/runtime.go`
- `internal/app/services/services.go`
- `internal/app/services/runtime.go`
- `internal/app/services/remote_install.go`

## Target architecture

### Before

Typical current trace, especially for `add` and `update`:

```txt
CLI command
  -> parse flags
  -> inspect target kind
  -> call one of several app service methods
  -> maybe call dry-run method instead
  -> maybe ensure dirs
  -> maybe acquire lock
  -> maybe prompt
  -> maybe show progress
  -> shape output manually
```

Example: `AddCmd` currently fans out into:

- `ResolveIntegrateTarget`
- `PlanLocalIntegration`
- `IntegrateLocal`
- `Reintegrate`
- `PlanDirectURLInstall`
- `InstallDirectURL`
- `PlanPackageRefInstall`
- `InstallPackageRef`

That means the CLI still understands too much of the workflow shape.

### After

Each command should have one dominant command trace:

```txt
CLI command
  -> request := parseXRequest(cmd, args)
  -> result, err := runtimeServicesFrom(cmd).X.Execute(ctx, request)
  -> renderXResult(cmd, result)
```

The application service owns the workflow decision tree. CLI owns only:

- flag/arg parsing
- prompts/progress adapters
- output rendering
- exit/error presentation

## Core refactor: introduce command-level use cases

Add command-shaped request/result interfaces in `internal/app/services`.

### Proposed service shape

Instead of service interfaces exposing many workflow fragments, move toward this shape:

```go
type AddService interface {
    Add(ctx context.Context, req AddRequest) (*AddResult, error)
}

type ListService interface {
    List(ctx context.Context, req ListRequest) (*ListResult, error)
}

type InfoService interface {
    Info(ctx context.Context, req InfoRequest) (*InfoResult, error)
}

type RemoveService interface {
    Remove(ctx context.Context, req RemoveRequest) (*RemoveResult, error)
}

type UpdateService interface {
    Update(ctx context.Context, req UpdateRequest) (*UpdateResult, error)
}

type SelfUpdateService interface {
    SelfUpdate(ctx context.Context, req SelfUpdateRequest) (*SelfUpdateResult, error)
}
```

This does not mean every internal helper disappears immediately. The important part is that normal CLI command files stop composing multiple service methods per command.

## Use a shared command runner in CLI

Introduce a small CLI-side runner/decorator for repeated command concerns.

Candidate file:

- `internal/cli/command_runner.go`

Responsibilities:

```go
type commandRunOptions struct {
    RequiresRuntimeDirs bool
    RequiresWriteLock   bool
}

func runCommand[T any](
    cmd *cobra.Command,
    opts commandRunOptions,
    execute func(context.Context) (*T, error),
    render func(*T) error,
) error
```

This runner can centralize:

- `mustEnsureRuntimeDirs`
- `withStateWriteLock`
- context fallback
- operation logging later, if useful

This removes repeated command boilerplate such as:

```go
if err := mustEnsureRuntimeDirs(); err != nil {
    return err
}

err = withStateWriteLock(cmd, func() error {
    // command work
})
```

Keep the runner in `internal/cli`, because locking and terminal/runtime preparation remain CLI/runtime concerns.

## Request/result design

### 1. `AddRequest`

Current `add` has the most scattered trace. Make the app service own target classification and mode selection.

```go
type AddRequest struct {
    Target AddTargetInput

    DryRun bool

    SHA256       string
    AssetPattern string

    ConfirmUpdateSourceReplace UpdateSourceReplaceConfirmer
    ResolvePackageAmbiguity    PackageViewAmbiguityResolver
}

type AddTargetInput struct {
    Positional string
    URL        string
    Provider   *ProviderRef
}
```

Result:

```go
type AddResult struct {
    Action AddAction
    Status string

    App     *AppDetails
    Plan    *DryRunPlan
    Package *PackageView

    AlreadyIntegrated bool
}
```

The app service decides:

- local AppImage integration
- reintegration of unlinked app
- already-integrated no-op
- direct URL install
- package ref install
- dry-run plan

CLI then renders based on `Action`/`Status`.

Expected CLI simplification:

```txt
AddCmd
  -> resolve add input only
  -> build AddRequest
  -> Add.Add(ctx, req)
  -> renderAddResult
```

Delete or reduce CLI-level functions over time:

- `runIntegrateTarget`
- `runInstallTarget`
- `runInstallPackageRef`

They can first become thin wrappers, then disappear.

### 2. `RemoveRequest`

`remove` is already close, but dry-run is still a separate service call.

Current:

- dry-run calls `PlanRemove`
- real run calls `Remove`
- command handles lock/dirs directly

Simplify to:

```go
type RemoveRequest struct {
    ID     string
    Unlink bool
    DryRun bool
}
```

Single app method:

```go
Remove(ctx, req) (*RemoveResult, error)
```

`RemoveResult` can contain either performed paths or planned paths.

This lets `RemoveCmd` become:

```txt
parse id/link
-> runCommand(requires lock unless dry-run)
-> Remove.Remove(ctx, req)
-> renderRemoveResult
```

### 3. `ListRequest`

Current `ListCmd` asks the app service for both integrated and unlinked apps, then performs filtering and grouping in CLI.

Move filter semantics into app service:

```go
type ListRequest struct {
    Filter ListFilter
}

type ListFilter string

const (
    ListAll        ListFilter = "all"
    ListIntegrated ListFilter = "integrated"
    ListUnlinked   ListFilter = "unlinked"
)
```

`ListResult` should return already-selected rows, plus optional counts if rendering needs section decisions.

CLI keeps only output-format rendering.

### 4. `InfoRequest`

Current `InfoCmd` chooses between:

- package ref info
- local AppImage info
- managed app info

That target classification should move into the app service.

```go
type InfoRequest struct {
    Input        string
    Provider     *ProviderRef
    AssetPattern string
}

type InfoResult struct {
    Kind InfoKind
    // existing info result fields
}
```

CLI still parses provider flags, but app service decides whether positional input is:

- managed app id
- local AppImage path
- invalid remote-looking input
- unknown target

This makes `InfoCmd` mirror `AddCmd`.

### 5. `UpdateRequest`

`update` should be refactored carefully because `internal/cli/update_workflow.go` currently owns a large workflow.

Current CLI responsibilities include:

- deciding check/apply/dry-run/check-only paths
- interpreting check rows
- prompting
- applying updates with lock
- mapping apply results back into rows
- rendering many statuses

Target app-level shape:

```go
type UpdateRequest struct {
    TargetID string

    Mode UpdateMode

    DryRun    bool
    AutoApply bool
    UseCache  bool

    Source *UpdateSourceRequest
}

type UpdateMode string

const (
    UpdateModeManagedCheckApply UpdateMode = "managed_check_apply"
    UpdateModeCheckOnly         UpdateMode = "check_only"
    UpdateModeSetSource         UpdateMode = "set_source"
    UpdateModeUnsetSource       UpdateMode = "unset_source"
)
```

Result:

```go
type UpdateResult struct {
    Mode UpdateMode

    Rows []UpdateRow

    Pending []ManagedUpdateView

    Applied  []ManagedApplyResultView
    Failures []ManagedCheckFailureView

    NeedsConfirmation bool
    ConfirmationLabel string
}
```

There are two good options for confirmation.

#### Option A: two-step app use case

```txt
Update.Check(ctx, req) -> pending result
CLI prompts
Update.Apply(ctx, applyReq) -> final result
```

This preserves CLI ownership of prompts, but keeps the app service responsible for check/apply result modeling.

#### Option B: application-owned workflow with CLI prompt port

```go
type UpdateRequest struct {
    // existing fields
    ConfirmApply Confirmer
    ReporterFor  ManagedApplyReporterFactory
}
```

This gives the simplest trace:

```txt
UpdateCmd -> Update.Update(ctx, req) -> render
```

Choose **Option B** because the project already uses app-layer callback seams like:

- `UpdateSourceReplaceConfirmer`
- `PackageViewAmbiguityResolver`
- `ManagedApplyReporterFactory`

So a `ConfirmApply` port is consistent, and the app layer still does not know terminal details.

### 6. `SelfUpdateRequest`

`self-update` currently has two app calls from CLI:

- `SelfUpdate.Check`
- `SelfUpdate.SelfUpdate`

Make self-update one use case:

```go
type SelfUpdateRequest struct {
    CurrentVersion string
    DryRun         bool // if useful later
}

type SelfUpdateResult struct {
    CurrentVersion   string
    LatestVersion    string
    InstalledVersion string
    UpToDate         bool
    Updated         bool
}
```

Then `runSelfUpdate` becomes:

```txt
SelfUpdate.SelfUpdate(ctx, req)
-> renderSelfUpdateResult
```

The app service owns “check first, self-update only if needed.”

## Runtime wiring simplification

`internal/cli/runtime.go` currently constructs a large `runtimeServices` value and still contains many global function variables, for example:

- `runAppUpdateCheck`
- `downloadRemoteAsset`
- `checkAimSelfUpdate`
- `runManagedApply`
- `integrateManagedUpdate`
- `integrateLocalApp`
- `removeManagedApp`
- `addAppsBatch`
- `addSingleApp`

Some are useful test seams, but the trace is harder to follow because behavior is split between globals, closures, app service structs, and runtime wiring.

### Refactor direction

Create a single runtime composition module that builds concrete app services from explicit ports.

Candidate shape:

```go
type appRuntime struct {
    Store       appservices.AppStore
    Discovery   appservices.DiscoveryService
    Downloader  appservices.Downloader
    Integrator  appservices.Integrator
    Remover     appservices.Remover
    Updater     appservices.Updater
    Locker      appservices.StateLocker
}

func newRuntimeServices(settings runtimeSettings) runtimeServices
```

Keep this in CLI/runtime wiring if it imports infrastructure. Do not move concrete infra construction into `internal/app`.

The important improvement: each command receives one service, and each service receives named ports, not scattered function globals.

## Recommended implementation order

### Phase 0: characterization tests

Before refactoring, add or strengthen command-level tests around current behavior.

Prioritize:

1. `add`
   - local AppImage dry-run
   - direct URL dry-run
   - package ref dry-run
   - reintegrate unlinked app
   - already integrated
2. `remove`
   - dry-run
   - unlink
   - remove with lock
3. `update`
   - check-only
   - dry-run
   - no pending updates
   - pending updates declined
   - pending updates applied
   - single target check failure
4. `info`
   - managed app
   - local AppImage
   - package ref
5. `list`
   - `--all`
   - `--integrated`
   - `--unlinked`
   - JSON/CSV/plain where relevant

Validation:

```sh
make test
make test-architecture
```

### Phase 1: add shared CLI runner

Add `internal/cli/command_runner.go`.

Refactor only `RemoveCmd` first because it is small and already service-shaped.

Expected result:

- less boilerplate in `RemoveCmd`
- no behavior change
- establishes pattern for lock/runtime-dir handling

Validation:

```sh
go test ./internal/cli
make test-architecture
```

### Phase 2: unify `RemoveService`

Change:

```go
Remove(ctx, req)
PlanRemove(ctx, req)
```

to one method:

```go
Remove(ctx, req RemoveRequest)
```

with `RemoveRequest.DryRun`.

Update tests and remove `PlanRemove` after call sites are gone.

This is the safest proof of the request/result pattern.

### Phase 3: simplify `ListService` and `ListCmd`

Move list filtering into `StoreListService`.

`ListCmd` should only:

1. validate mutually exclusive flags
2. convert flags into `ListFilter`
3. call `List`
4. render selected rows

This reduces CLI business-ish grouping logic and gives a clean example for read-only commands.

### Phase 4: simplify `SelfUpdateService`

Collapse check + self-update into one app use case.

Current CLI `runSelfUpdate` has self-update decision logic. Move that into `SelfUpdateWorkflowService`.

Target:

```go
result, err := runtimeServicesFrom(cmd).SelfUpdate.SelfUpdate(ctx, appservices.SelfUpdateRequest{
    CurrentVersion: version,
})
return renderSelfUpdateResult(cmd, result)
```

Keep busy indicators in CLI either by:

- wrapping the whole call once, or
- giving `SelfUpdateRequest` an app-defined progress reporter

Prefer one busy indicator initially.

### Phase 5: simplify `InfoService`

Introduce single:

```go
Info(ctx, req InfoRequest) (*InfoResult, error)
```

Keep existing `ManagedAppInfo`, `LocalAppImageInfo`, and `PackageRefInfo` temporarily as private helpers or methods on the implementation.

Then shrink `InfoCmd` to request construction plus rendering.

### Phase 6: consolidate `AddService`

This is the highest-value refactor after `update`.

Steps:

1. Add `Add(ctx, AddRequest)`.
2. Implement it by delegating to existing methods internally.
3. Change `AddCmd` to call only `Add`.
4. Move mode-specific result shaping into app result types.
5. Delete public service methods once no CLI code calls them:
   - `ResolveIntegrateTarget`
   - `IntegrateLocal`
   - `Reintegrate`
   - `InstallDirectURL`
   - `InstallPackageRef`
   - `PlanLocalIntegration`
   - `PlanDirectURLInstall`
   - `PlanPackageRefInstall`

The implementation may still have private helpers with these names. The simplification is at the module interface.

This makes `BasicAddService` deeper: one small interface hides a larger workflow.

### Phase 7: consolidate direct URL/package install internals

After `AddService` is simplified, revisit:

- `BasicAddService`
- `RemoteInstallService`

Current split is somewhat shallow:

```txt
BasicAddService.InstallDirectURL
  -> RemoteInstallService.InstallDirectURL
```

and:

```txt
BasicAddService.InstallPackageRef
  -> Discovery
  -> RemoteInstallService.InstallPackageMetadata
```

Possible consolidation:

```go
type AddWorkflowService struct {
    Store      AppStore
    Discovery  DiscoveryService
    Installer  RemoteInstaller
    Integrator LocalIntegrator
}
```

This gives one app-level add workflow module with clear private helpers:

- `addLocal`
- `reintegrate`
- `installDirectURL`
- `installPackageRef`
- `plan`

Do this only after Phase 6, because the command trace simplification gives most of the benefit.

### Phase 8: refactor `update`

Do this last.

Recommended steps:

1. Introduce `Update(ctx, UpdateRequest) (*UpdateResult, error)` while keeping existing methods.
2. Move `prepareManagedUpdateRun` semantics into `UpdateRequest` construction plus app service defaults.
3. Move `collectManagedUpdateRows` into `SourceUpdateService`.
4. Move apply/result-row reconciliation into `SourceUpdateService`.
5. Keep render functions in `internal/cli/update_workflow.go`.
6. Replace `runManagedUpdate` with:

   ```go
   req, err := parseUpdateRequest(cmd, args)
   result, err := runtimeServicesFrom(cmd).Update.Update(ctx, req)
   return renderUpdateResult(cmd, result)
   ```

7. Delete now-private CLI workflow helpers.

Important: CLI should still own:

- output formatting
- JSON/CSV/plain rendering
- terminal prompts via an app-defined confirmation interface
- progress reporters

App should own:

- update mode semantics
- check/apply orchestration
- cache use
- result row statuses
- persistence/cache invalidation coordination

## Suggested final command trace

After the refactor, every command should read like this:

```go
func RemoveCmd(cmd *cobra.Command, args []string) error {
    req, runOpts, err := parseRemoveRequest(cmd, args)
    if err != nil {
        return err
    }

    return runCommand(cmd, runOpts, func(ctx context.Context) (*appservices.RemoveResult, error) {
        return runtimeServicesFrom(cmd).Remove.Remove(ctx, req)
    }, func(result *appservices.RemoveResult) error {
        return renderRemoveResult(cmd, result)
    })
}
```

For complex commands like `add` and `update`, the same shape should hold, even if request parsing/rendering is larger.

## Acceptance criteria

The refactor is successful when:

1. Each top-level command calls one dominant service method.
2. Dry-run is modeled in the same request/result path, not as a separate service trace.
3. CLI no longer chooses between multiple app workflow methods for the same command.
4. `internal/app/services` exposes command-shaped interfaces.
5. `internal/cli` owns rendering and terminal interaction only.
6. State lock and runtime-dir handling are centralized.
7. Architecture check still passes without weakening rules.
8. Existing command behavior is covered by tests.

Validation per phase:

```sh
make fmt
go test ./internal/cli ./internal/app/services
make test-architecture
```

For larger phases, especially `add` and `update`:

```sh
make check
```

## Top recommendation

Start with **`remove`**, then **`list`**, then **`self-update`**.

They are small enough to establish the new trace safely. Once the pattern is proven, apply it to **`add`**, and only then tackle **`update`**, which has the most workflow complexity and the highest regression risk.
