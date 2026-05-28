# Agent Notes for AppImage Manager

## Project Context

AppImage Manager (`aim`) is a Go CLI for managing AppImages: discovery, integration,
removal, updates, upgrades, and related desktop integration behavior.

The project is still in beta. Backwards compatibility is not required.

## Compatibility Policy

- We are still in beta; backwards compatibility is not required.
- Changes are allowed to break existing behavior when they simplify or improve the design.
- When refactoring, do **not** add compatibility shims.
- Prefer direct migrations/refactors over preserving old internal APIs.

## Commands

Prefer the `Makefile` targets for local development tasks. The raw commands are documented
for CI parity and troubleshooting.

### Make targets

```sh
make build
make build-version VERSION=0.0.0-dev
make run
make test
make test-architecture
make vet
make fmt
make check
make clean
```

### Build

```sh
make build
# equivalent: go build -o ./bin/aim ./cmd/aim
```

Build with embedded version:

```sh
make build-version VERSION=0.0.0-dev
# equivalent: go build -ldflags "-X main.version=0.0.0-dev" -o ./bin/aim ./cmd/aim
```

### Run

```sh
make run
# equivalent: go run ./cmd/aim --help
```

### Test

```sh
make test
# equivalent: go test ./...
```

Run one package:

```sh
go test ./internal/app/integrate
go test ./cmd/aim
```

### Static checks

No dedicated third-party linter is configured. Use Go standard tooling.

```sh
make vet
# equivalent: go vet ./...
```

### Format

Format touched Go files before finalizing changes:

```sh
make fmt
# equivalent: gofmt -w ./cmd ./internal
```

### Quality pass

Before finalizing non-trivial changes, run:

```sh
make check
```

### GoReleaser

Check GoReleaser config:

```sh
goreleaser check
```

Create a local snapshot release without publishing:

```sh
goreleaser release --snapshot --clean
```

Official releases are run by GitHub Actions with:

```sh
goreleaser release --clean
```

## Architecture

This project uses a layered architecture.

```txt
cmd/aim
  -> internal/cli          presentation layer
  -> internal/app          application/use-case layer
  -> internal/domain       domain model and rules
  -> internal/infra        infrastructure adapters
  -> internal/architecture architecture tests
```

### Responsibilities

#### `cmd/aim`

- Application entrypoint.
- Bootstrap and dependency wiring.
- Version injection.
- Should stay minimal.
- Should not contain business logic.

#### `internal/cli`

- CLI commands, flags, arguments, help text.
- User-facing stdout/stderr behavior.
- Exit-code mapping.
- Calls application services/use cases.
- Should not contain business logic.
- Should not perform infrastructure work directly when an app service or port should own it.

#### `internal/app`

Application/use-case layer.

Typical responsibilities:

- Orchestrating workflows.
- Defining ports/interfaces needed from infrastructure.
- Coordinating domain rules and infrastructure adapters.
- Returning errors rather than exiting the process.
- Keeping behavior testable with fakes.

Current app areas include:

```txt
internal/app/appimage
internal/app/clock
internal/app/discovery
internal/app/integrate
internal/app/remove
internal/app/services
internal/app/update
internal/app/upgrade
```

#### `internal/domain`

- Domain entities, value objects, and business rules.
- Framework-independent.
- No CLI, filesystem, HTTP, process, or desktop-environment concerns.
- Should not import `internal/app`, `internal/infra`, or `internal/cli`.

#### `internal/infra`

Infrastructure adapters.

Typical responsibilities:

- Filesystem access.
- HTTP clients/downloads.
- GitHub API/release access.
- Desktop files/icons.
- Config persistence.
- Repository/storage implementations.
- AppImage-specific low-level integration with the host system.
- Implements interfaces defined by `internal/app`.

Current infra areas include:

```txt
internal/infra/appimage
internal/infra/config
internal/infra/desktop
internal/infra/download
internal/infra/filesystem
internal/infra/github
internal/infra/httpclient
internal/infra/repository
internal/infra/selfupdate
internal/infra/zsync
```

#### `internal/architecture`

- Architecture tests.
- Enforces import boundaries between layers.
- Do not weaken these tests to make a refactor easier.

## Dependency Rules

The intended dependency direction is inward toward application/domain logic.

Preferred:

```txt
cmd/aim -> cli
cmd/aim -> app
cmd/aim -> infra

cli -> app
app -> domain
infra -> app
infra -> domain
```

Forbidden:

```txt
domain -> app
domain -> infra
domain -> cli

app -> infra
app -> cli

infra -> cli
```

Dependency injection and concrete adapter wiring should happen at the edge, primarily in
`cmd/aim` and CLI/runtime wiring code.

These boundaries are enforced by:

```txt
internal/architecture/import_boundaries_test.go
```

If a change violates the architecture test, fix the dependency direction instead of
weakening the test.

## Application Design Conventions

- Keep command handlers thin.
- Put orchestration in `internal/app`.
- Put external-system details in `internal/infra`.
- Define interfaces at the consumer side, usually in `internal/app`.
- Prefer small interfaces tailored to a use case.
- Return errors instead of calling `os.Exit` outside the entrypoint.
- Use `context.Context` for blocking operations, HTTP, filesystem-heavy workflows, and process execution.
- Prefer standard library packages unless a dependency adds clear value.
- Prefer composition over global mutable state.
- Avoid package-level mutable state unless there is a strong reason.
- Keep behavior deterministic and easy to test.

## Go Conventions

- Keep packages small and focused around a clear responsibility.
- Prefer explicit dependencies passed through constructors or function parameters.
- Keep interfaces close to the consumer, usually in `internal/app`.
- Use descriptive names; avoid abbreviations unless they are common Go conventions.
- Keep exported APIs minimal and document exported identifiers when they are part of package contracts.
- Use table-driven tests when they improve readability.
- Use `errors.Is`/`errors.As` for sentinel or typed error handling when appropriate.
- Avoid panics in application code; return errors instead.
- Run `make fmt` before finalizing Go changes.

## Git Conventions

- Use focused commits that group related changes.
- Prefer Conventional Commit-style messages:

```txt
<type>(optional-scope): <short imperative summary>
```

Common types:

```txt
feat:     user-facing feature
fix:      bug fix
refactor: code restructuring without behavior change
test:     test-only changes
docs:     documentation-only changes
build:    build system or dependency changes
ci:       CI workflow changes
chore:    maintenance tasks
```

Examples:

```txt
feat(integrate): add desktop entry validation
fix(update): handle missing release assets
refactor(app): split update ports by use case
docs: expand agent instructions
build: add local Makefile targets
```

- Keep commit subjects concise, imperative, and lowercase after the type.
- Do not mix unrelated refactors, behavior changes, and formatting-only changes in one commit when avoidable.
- Do not create commits unless explicitly asked by the user.

## Error Handling

- Return descriptive errors with context.
- Wrap errors where it helps callers understand the failed operation.
- Do not print and return the same error from lower layers.
- CLI should decide how errors are presented to users.
- Application and domain layers should not write directly to stdout/stderr.

## Testing

- Add or update tests for behavior changes.
- Prefer table-driven tests where they improve clarity.
- Test domain behavior directly.
- Test application services with fake ports/interfaces.
- Test CLI behavior through command execution or command-level helpers.
- Keep infrastructure tests focused on adapter behavior and edge cases.
- Run architecture tests as part of `go test ./...`.

Useful commands:

```sh
make test
go test ./internal/app/integrate
make test-architecture
```

## Agent Rules

When modifying this repository:

- Do not put business logic in `internal/cli` or `internal/infra`.
- Do not bypass the `internal/app` layer from CLI code.
- Keep `cmd/aim/main.go` minimal.
- Respect the architecture boundaries enforced by `internal/architecture`.
- Do not add backwards-compatibility shims for old internal behavior.
- Do not bypass GoReleaser for official release packaging.
- Keep `.goreleaser.yaml`, `.github/workflows/release.yml`, and `scripts/release-prepare.sh` in sync when changing release artifacts.
- If changing CLI flags, commands, help text, completions, or man-page generation, consider testing the release preparation path.
- Prefer focused refactors over broad rewrites.
- Keep changes consistent with existing package style.
- Add tests for behavior changes.
- Run `make fmt` on touched Go files.
- Run `make test` for meaningful code changes.
- Run `make vet` before release-related or broad refactor work.
- Run `make check` before finalizing non-trivial changes.

## Release Flow

Official releases are published by GitHub Actions using GoReleaser.

Release workflow:

```txt
git tag vX.Y.Z
  -> GitHub Actions release workflow
  -> verifies CI passed for the tagged commit
  -> runs goreleaser check
  -> runs goreleaser release --clean
```

Release config lives in:

```txt
.goreleaser.yaml
.github/workflows/release.yml
scripts/release-prepare.sh
```

GoReleaser builds Linux artifacts for:

```txt
linux/amd64
linux/arm64
```

The release build embeds the version with:

```sh
-X main.version={{ .Version }}
```

Before creating a release tag, run:

```sh
make check
goreleaser check
```

Optionally verify release packaging locally with:

```sh
goreleaser release --snapshot --clean
```

Official releases are triggered by pushing a SemVer tag matching:

```txt
vX.Y.Z
```

Example:

```sh
git tag v1.2.3
git push origin v1.2.3
```

The release workflow requires the tagged commit to already have a successful CI push run on `main`.

The GoReleaser `before` hook runs:

```sh
scripts/release-prepare.sh "{{ .Tag }}"
```

That script generates and validates release assets, including:

- man page
- bash completion
- zsh completion
- fish completion

Do not manually edit generated release assets in `dist/`; they are prepared during release.
