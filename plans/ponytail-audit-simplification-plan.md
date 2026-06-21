# Ponytail Audit Simplification Plan

This plan is grouped by logical commits. Implement one checklist item at a time.

**Agent rule:** after implementing an item and validating it, edit this file and change that item from `[ ]` to `[x]` before starting the next item. Do not skip ahead. Do not check off an item until the code change is complete and tests/builds relevant to that item have been run.

## Commit 1: `refactor(storage): remove legacy migration support`

- [x] Delete the v1 migration implementation and tests.
  - Remove `internal/infra/migration/v1.go`.
  - Remove `internal/infra/migration/v1_test.go`.
  - Remove the `internal/infra/migration` import and `migration.MigrateV1` call from `cmd/aim/main.go`.
  - Validate with `go test ./...`.

- [x] Remove legacy storage decoding shims.
  - In `internal/infra/storage/repository.go`, delete:
    - `sourceRecord.UnmarshalJSON`
    - `updateSourceRecord.UnmarshalJSON`
    - `updateSourceRecordFromLegacyString`
    - `validLegacyGitHubRepo`
  - Keep only the current `schema_version: 2` JSON shape.
  - Update or delete tests that only cover legacy compatibility.
  - Validate with `go test ./internal/infra/storage ./...` if practical.

## Commit 2: `build(config): remove toml config loading`

- [x] Remove TOML config loading and dependency.
  - Delete `internal/infra/config/loader.go` and `internal/infra/config/loader_test.go`, or replace them with tiny XDG-default helpers if still needed.
  - Remove `github.com/pelletier/go-toml/v2` from `go.mod` via `go mod tidy`.
  - Update `cmd/aim/main.go` to use XDG defaults directly.
  - Keep the `paths` command working if it is still useful.
  - Validate with `go test ./...` and `go mod tidy`.

- [x] Simplify `app.Config` to only fields still used.
  - Review `internal/app/config.go` and remove fields made redundant by config deletion.
  - Update `PathsResult` and `paths` command only if fields truly disappear.
  - Validate with `go test ./internal/app ./internal/cli/... ./...` if practical.

## Commit 3: `refactor(storage): simplify file locking`

- [ ] Remove `unix.Flock` and lock-file machinery.
  - In `internal/infra/storage/repository.go`, remove the `golang.org/x/sys/unix` dependency.
  - Delete `repositoryLocks`, `repositoryLock`, and lock-file open/unlock code.
  - Either rely on atomic temp-write + rename, or keep a single simple package-level `sync.Mutex` if in-process serialization is still wanted.
  - Validate storage tests with `go test ./internal/infra/storage`.

## Commit 4: `refactor(activity): replace live progress renderer with simple output`

- [ ] Replace spinner/progress-bar renderer with a simple activity reporter.
  - In `internal/cli/activity/reporter.go`, remove goroutine ticking, terminal clearing, terminal width detection, progress layout, bar rendering, and ANSI cursor movement.
  - Keep app-layer `ActivityReporter` semantics if useful, but render simple start/done/fail messages only.
  - Delete local duplicate `noopTask` if app-layer `NoopActivityTask` can be reused.
  - Validate CLI/activity tests if present and `go test ./internal/cli/...`.

- [ ] Drop `golang.org/x/sys` after storage and activity no longer need it.
  - Run `go mod tidy`.
  - Confirm `go.mod` no longer lists `golang.org/x/sys`.
  - Validate with `go test ./...`.

## Commit 5: `refactor(infra): share file copy and removal helpers`

- [ ] Add one shared infra file-copy helper.
  - Create a small shared helper package or file under `internal/infra`, for atomic copy using temp file + `io.Copy` + `os.Rename`.
  - Replace duplicated `copyFile` in `internal/infra/appimage/installer.go`.
  - Replace duplicated `copyIconFile` in `internal/infra/icon/installer.go`.
  - Keep behavior such as permissions and owner-executable handling intact.
  - Validate with `go test ./internal/infra/appimage ./internal/infra/icon`.

- [ ] Collapse identical artifact removers.
  - Replace `AppImageRemover`, `IconRemover`, and `DesktopEntryRemover` with one simpler removal path.
  - Prefer a single app-layer artifact-removal port if keeping clean architecture boundaries.
  - Delete the three near-identical remover adapters:
    - `internal/infra/appimage/remover.go`
    - `internal/infra/icon/remover.go`
    - `internal/infra/desktop/remover.go`
  - Update `ServiceDeps`, service fields, tests, and `cmd/aim/main.go`.
  - Validate with `go test ./internal/app ./internal/infra/...`.

## Commit 6: `refactor(workspace): inline temporary workspace handling`

- [ ] Remove `WorkspaceProvider` abstraction.
  - Delete `internal/app/workspace.go` if no longer needed.
  - Delete `internal/infra/workspace/provider.go` and tests.
  - In app workflows, use `os.MkdirTemp("", "aim-*")` and `defer os.RemoveAll(path)` directly, or a small unexported app helper.
  - Update tests to stop faking workspace providers.
  - Validate with `go test ./internal/app ./...` if practical.

- [ ] Remove `AppImageStager` abstraction if it only copies into a temp dir.
  - Delete `AppImageStager` from `internal/app/integration_ports.go`.
  - Delete `internal/infra/appimage/stager.go` and tests if no longer needed.
  - Use the shared file-copy helper or integrate directly from the original path if staging is unnecessary.
  - Update `ServiceDeps`, service fields, tests, and `cmd/aim/main.go`.
  - Validate with `go test ./internal/app ./internal/infra/appimage`.

## Commit 7: `refactor(app): flatten add-local workflow helpers`

- [ ] Replace the add-local helper call ladder.
  - In `internal/app/service.go`, collapse:
    - `addLocalWithSource`
    - `addLocalWithSourceAndID`
    - `addLocalWithSourceAndIDAndSave`
  - Use one helper with either direct parameters or a small options struct, for example:

    ```go
    type addLocalOptions struct {
        source          domain.Source
        fallbackVersion string
        appID           string
        saveApp         bool
    }
    ```

  - Update all call sites.
  - Validate with `go test ./internal/app`.

## Commit 8: `refactor(app): simplify service interfaces`

- [ ] Remove per-command service capability interfaces.
  - In `internal/app/service_ports.go`, remove command-only interfaces like `Adder`, `Remover`, `Updater`, `IDManager`, `Lister`, `Informer`, `SelfUpdateRunner`, and `PathProvider` unless a specific boundary still needs them.
  - Keep one `Service` interface or return/use the concrete service where simpler.
  - Update CLI command constructors to accept the simplified service type.
  - Update tests and fakes accordingly.
  - Validate with `go test ./internal/app ./internal/cli/...`.

## Commit 9: `refactor(infra): remove production test-injection knobs`

- [ ] Simplify adapter structs that expose test-only knobs.
  - `internal/infra/download/downloader.go`: prefer `http.DefaultClient` directly unless a real runtime override exists.
  - `internal/infra/desktop/refresher.go`: remove public `LookPath` and `Run` fields; use `exec.LookPath` and `exec.CommandContext` internally.
  - `internal/infra/selfupdate/installer.go`: remove public `ScriptURL`, `HTTPClient`, and private `runCommand` if they only exist for unit tests.
  - `internal/infra/workspace/provider.go`: already removed by Commit 6; if not, remove configurable `Pattern`.
  - Update tests to use `httptest.Server`, temporary PATH scripts, or higher-level tests instead of production injection fields.
  - Validate with `go test ./internal/infra/...`.

## Commit 10: `refactor(cli): centralize yes-no prompts`

- [ ] Add a shared CLI confirmation helper.
  - Add a small helper, e.g. `internal/cli/prompt.ConfirmYesNo(ctx, in, out, question, autoConfirm)`.
  - Replace duplicated prompt logic in:
    - `internal/cli/command/update/command.go`
    - `internal/cli/command/selfupdate/command.go`
  - Keep JSON mode auto-confirm behavior unchanged.
  - Validate with `go test ./internal/cli/command/update ./internal/cli/command/selfupdate`.

## Commit 11: `refactor(cli): reduce output dto duplication`

- [ ] Simplify JSON DTO mapping for simple commands.
  - For `list`, `paths`, and other simple commands, encode app result structs directly if their JSON shape is acceptable.
  - Keep anonymous structs only when CLI JSON names intentionally differ from app field names.
  - Validate command tests.

- [ ] Centralize or shrink `info` source/update-source JSON mapping.
  - Either move source/update-source DTO mapping to a shared CLI output helper, or reduce it to only fields the command actually needs.
  - Avoid duplicating persistence DTOs in CLI.
  - Validate `go test ./internal/cli/command/info`.

## Final validation

- [ ] Run `go mod tidy`.
- [ ] Run `go test ./...`.
- [ ] Review `go.mod` and confirm possible dependency removals:
  - `github.com/pelletier/go-toml/v2`
  - `golang.org/x/sys`
- [ ] Run a quick manual smoke test if practical:
  - `go run ./cmd/aim --help`
  - `go run ./cmd/aim paths`
- [ ] Confirm every completed item in this file is checked off.
