# AppImage Manager - `aim`

> Manage AppImages from the command line.

[![GitHub Release](https://img.shields.io/github/v/release/slobbe/appimage-manager?sort=semver&display_name=release&style=flat-square&color=royalblue)](https://github.com/slobbe/appimage-manager/releases/latest)
[![GitHub License](https://img.shields.io/github/license/slobbe/appimage-manager?style=flat-square&color=teal)](/LICENSE)

> [!WARNING]
> This project is still a work in progress.
> Breaking changes may occur while it remains in **v0.x.x**.

## Features

- Install AppImages from local files, direct URLs, GitHub, and GitLab
- Integrate apps with desktop menus, icons, and launchers
- Track managed apps and update them from configured sources
- Inspect AppImage metadata and update-source details
- Remove apps, unlink desktop integration, and run repair or migration when needed

## Installation

```sh
curl -fsSL https://raw.githubusercontent.com/slobbe/appimage-manager/main/scripts/install.sh | sh
aim --version
```

If `aim` is not found, make sure `~/.local/bin` is on your `PATH`.

## Quickstart

### `add`

Add an AppImage from a local file, a direct download link, or a GitHub/GitLab release.

```sh
# Examples
aim add ./example.AppImage
aim add https://example.com/example.AppImage
aim add --github owner/repo
```

### `info`

Get information about a managed app, local AppImage file, or remote.

```sh
# Examples
aim info example-app
aim info ./example.AppImage
aim info --github owner/repo
```

### `update`

Check for updates, apply them, or configure an update source.

```sh
# Examples
aim update
aim update --check-only
aim update set example-app --github owner/repo
```

### `remove`

Remove a managed app or unlink its desktop integration.

```sh
# Examples
aim remove example-app
aim remove --unlink example-app
```

### Other useful commands

```sh
aim list        # list all managed AppImages
aim --upgrade   # upgrade aim to the newest version
```

Use `aim --help`, `aim <command> --help`, or the man page for full option details.

## Global flags for scripting

`aim` now exposes a consistent set of global flags on all visible commands:

- `-h`, `--help`: built-in command help
- `-v`, `--version`: print the CLI version
- `--verbose`: emit diagnostic logs on stderr
- `-q`, `--quiet`: suppress non-essential status output
- `-C`, `--config <dir>`: use an alternate AIM state root
- `--dry-run`: preview mutating actions without applying them
- `-y`, `--yes`: bypass confirmation prompts
- `--output text|json|csv`: choose human-readable or machine-readable output

Examples:

```sh
aim list --output json
aim update --check-only --output csv
aim -C /tmp/aim-state add --dry-run ./Example.AppImage
aim update unset example-app --yes
```

## Output and exit status

`aim` keeps its process interface script-friendly:

- primary command output, including `--output json` and `--output csv`, is written to stdout
- errors, warnings, prompts, progress, and verbose diagnostics are written to stderr
- success exits with `0`; failures exit with a stable non-zero code

Exit codes:

- `0`: success
- `64`: invalid command usage
- `66`: requested local input or resource not found
- `69`: external service or tool unavailable
- `70`: internal or uncategorized software failure
- `73`: local write, create, or update failure
- `75`: temporary or retryable runtime failure
- `77`: permission, confirmation, or precondition refusal

## Where `aim` stores files

`aim` uses XDG base directories:

- App files: `${XDG_DATA_HOME:-~/.local/share}/aim`
- Desktop links: `${XDG_DATA_HOME:-~/.local/share}/applications`
- Desktop icons: `${XDG_DATA_HOME:-~/.local/share}/icons/hicolor` and `${XDG_DATA_HOME:-~/.local/share}/pixmaps`
- Config files: `${XDG_CONFIG_HOME:-~/.config}/aim`
- Database: `${XDG_STATE_HOME:-~/.local/state}/aim/apps.json`
- Temporary files: `${XDG_CACHE_HOME:-~/.cache}/aim/tmp`

## Development Notes

Build from source:

```sh
git clone https://github.com/slobbe/appimage-manager.git
cd appimage-manager
go build ./cmd/aim
```

Regenerate the committed man page:

```sh
go run -tags docgen ./cmd/aim
```

GoReleaser is the canonical release tool. To validate the release configuration locally without publishing:

```sh
goreleaser release --snapshot --clean --skip=publish,validate
```

## License

[MIT](/LICENSE)
