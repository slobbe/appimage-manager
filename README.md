# AppImage Manager (`aim`)

A CLI to install, integrate, inspect, and update AppImages on Linux.

![](https://shieldcn.dev/group/github/release/slobbe/appimage-manager+github/license/slobbe/appimage-manager.svg?variant=branded&size=sm)

> [!NOTE]
> This project is still a **work in progress** and breaking changes may happen at any time while in `v0.x.x`.

## Install

Install the latest release:

```sh
curl -fsSL https://raw.githubusercontent.com/slobbe/appimage-manager/main/scripts/install.sh | sh
aim --version
```

Install a specific version:

```sh
curl -fsSL https://raw.githubusercontent.com/slobbe/appimage-manager/main/scripts/install.sh | AIM_VERSION=v0.17.0 sh
```

`AIM_VERSION` accepts either `0.17.0`, `v0.17.0`, or a prerelease tag such as `v0.17.0-rc.1`.

The installer places `aim` in `~/.local/bin` by default and generates man pages and shell completions locally from the installed binary.

If `aim` is not found, make sure `~/.local/bin` is on your `PATH`.

## Common commands

### Add an AppImage

```sh
aim add ./Example.AppImage
aim add --github owner/repo
aim add --github owner/repo --asset '*x86_64.AppImage'
aim add --github owner/repo --prerelease
```

### Check and apply updates

```sh
aim update --check
aim --json update --check
aim update
aim update example-app
```

`aim update --check` reports available updates without modifying installed AppImages. Today, `aim update` only checks and applies GitHub release update sources; embedded `zsync`, `local_file`, and unsupported update metadata is preserved for inspection but not applied yet.

### Set or clear an update source

```sh
aim update --set example-app --github owner/repo
aim update --set example-app --github owner/repo --asset '*x86_64.AppImage'
aim update --set example-app --github owner/repo --prerelease
aim update --set example-app --embedded
aim update --unset example-app
```

Use `--asset` with Go `filepath.Match`-style patterns when a GitHub release has multiple AppImage assets, such as different architectures or flavors. `--embedded` preserves update metadata found inside the AppImage, but only embedded GitHub release sources are applied by `aim update` today.

### Remove an AppImage

```sh
aim remove example-app
```

### Inspect, list, and locate data

```sh
aim info example-app
aim info ./Example.AppImage
aim list
aim paths
```

`aim info <path>` inspects a local AppImage before integration. Inspection executes the AppImage's extraction/update-info modes to read metadata; inspect only AppImages you trust.

### Update aim itself

```sh
aim selfupdate
aim selfupdate --prerelease
```

## Useful commands and aliases

```sh
aim list      # list managed AppImages
aim remove    # remove a managed AppImage
aim update    # check for and apply app updates
aim info      # inspect an AppImage or integrated app
aim paths     # show aim's config/storage/cache paths
```

## Global flags

- `--json`: emit machine-readable JSON where supported; this does not confirm
  mutations
- `--yes`: automatically confirm mutation prompts
- `--non-interactive`: fail instead of prompting for input; combine with
  `--yes` to approve mutations in automation
- `--version`: print the current aim version

## More help

- `aim --help` for the CLI overview
- `aim help <command>` for command-specific manual pages
- `aim <command> --help` for flags and usage on a specific command
- `man aim` for the full manual page

## Roadmap

The roadmap prioritizes reliability and trust before expanding the number of
acquisition sources or adding a graphical interface.

### Phase 1: predictable and recoverable core

- separate output formatting from confirmation with `--json`, `--yes`, and
  `--non-interactive`
- define exit codes and JSON statuses for no-op, cancellation, skipped work,
  partial failure, and failure
- report missing and unsupported update sources explicitly
- make color terminal-aware and honor `NO_COLOR`
- strengthen update and ID-change transactions, including rollback failure
  reporting and committed-with-warning outcomes
- prevent concurrent `aim` processes from silently losing state updates
- add explicit HTTP timeouts and GitHub rate-limit diagnostics
- add black-box CLI tests covering output, exit status, cancellation, and
  temporary XDG environments
- document configuration, support boundaries, and recovery basics

### Phase 2: trustworthy update engine

- introduce a common update-provider interface and implement `zsync`
- support local-file updates for development and controlled environments
- persist and display update verification policy, with optional digest or
  signature verification
- replace script-based self-update with verified, atomic binary replacement
- retain the previous version after an update and support pinning and rollback
- add GitHub authentication and improved rate-limit handling

### Phase 3: diagnosis and lifecycle completion

- add `doctor`, `repair`, `clean`, and update history workflows
- detect missing files, broken records, orphaned artifacts, invalid update
  metadata, and stale staging files
- add dry-run support to destructive operations
- make common partial failures recoverable without manually editing `apps.json`

### Phase 4: broader acquisition and customization

- add direct HTTPS URL installation with optional SHA-256 verification
- support initial metadata overrides such as app ID, name, and icon
- discover and import existing AppImages
- support multiple update targets and include/exclude filters
- add GitLab or a generic release-feed adapter

### Phase 5: 1.0 readiness

- stabilize the JSON schema, exit-code contract, and persisted-state migrations
- publish signed releases, SBOMs, and build provenance
- smoke-test release archives and representative AppImages
- publish a support matrix, security policy, and recovery/release runbooks
- define compatibility, rollback, and corruption-recovery guarantees

## License

[MIT](/LICENSE)
