# Agent Notes

## Project Context

AppImage Manager (`aim`) is a Go CLI for managing AppImages: integration, removal, updates, self-updates, and related desktop integration behavior.

The project is still in beta. Backwards compatibility is not required.

## Architecture

This project uses a layered architecture.

```txt
cmd/aim                   composition root / entrypoint
  -> internal/cli         presentation layer
    -> internal/app       application/use-case layer
      -> internal/domain  domain model and rules

internal/infra            infrastructure adapters that implement app ports
  -> internal/app
```

## Dependency Rules

Architecture decision: use a Clean Architecture dependency direction with app-defined ports and infrastructure adapters wired at the composition root.

Allowed:

```txt
cmd/aim -> internal/cli
cmd/aim -> internal/app
cmd/aim -> internal/infra

internal/cli -> internal/app

internal/app -> internal/domain

internal/infra -> internal/app
internal/infra -> internal/domain
```

Forbidden:

```txt
internal/domain -> internal/app
internal/domain -> internal/infra
internal/domain -> internal/cli

internal/app -> internal/infra
internal/app -> internal/cli

internal/cli -> internal/infra
internal/cli -> internal/domain

internal/infra -> internal/cli
```

## Layer Rules

### CLI Layer Rules

`internal/cli` is responsible only for:

- parsing commands, arguments, and flags
- converting CLI input into `internal/app` request types
- calling `internal/app.Service`
- formatting human-readable and JSON output
- implementing terminal prompts and activity rendering for app-defined interfaces

`internal/cli` must not:

- perform AppImage integration/removal/update logic
- read or write application state directly
- call GitHub or network APIs directly
- import `internal/infra`
- import `internal/domain`

### App Layer Rules

`internal/app` owns use-case workflows and request/result contracts.

The app layer may define interfaces for external concerns, such as:

- storage
- filesystem operations
- GitHub/release lookup
- downloads
- progress/activity reporting
- confirmation prompts

Concrete implementations belong outside the app layer:

- terminal prompt/activity implementations live in `internal/cli`
- filesystem/network/config implementations live in `internal/infra`

Activity and prompt wording is a presentation concern.

The app layer should report semantic activity through structured fields, such as activity kind, app ID, repo, asset name, path, total bytes, etc. The CLI layer chooses the exact user-facing text.

### Domain Layer Rules

`internal/domain` contains pure domain models and rules.

`internal/domain` must not:

- perform filesystem or network I/O
- know about CLI formatting, prompts, colors, or JSON
- know about config file formats or XDG paths
- import `internal/app`, `internal/cli`, or `internal/infra`

### Infra Layer Rules

`internal/infra` contains adapters for external systems, such as:

- filesystem operations
- XDG path resolution
- config loading and TOML parsing
- GitHub/release APIs
- downloads
- desktop entry/icon integration

Infra implements app-defined interfaces and is wired from `cmd/aim`.

`internal/infra` may import `internal/domain` to persist and hydrate domain models, but must not put business rules in infrastructure adapters.

`internal/infra` must not import `internal/cli` or directly perform terminal prompts/progress rendering.

## Changelog

- Keep entries concise and short, with one bullet per feature, fix, or change.
- Focus on behavior useful to users, not implementation details.
- Group entries under `New`, `Changed`, and `Fixed` (in this order); omit empty groups.
- Write changelog entries in the past tense.
