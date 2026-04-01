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

Bare `aim` prints concise getting-started help. Use `aim --help` for the full inline reference and `aim help [command]` for the terminal manual page.

## Global flags for scripting

`aim` now exposes a consistent set of global flags on all visible commands:

- `-h`, `--help`: built-in command help
- `-v`, `--version`: print the CLI version
- `--verbose`: emit diagnostic logs on stderr
- `-q`, `--quiet`: suppress non-essential status output
- `-C`, `--config <dir>`: use an alternate AIM state root
- `--dry-run`: preview mutating actions without applying them
- `-y`, `--yes`: bypass confirmation prompts
- `--json`: emit machine-readable JSON
- `--csv`: emit CSV where supported
- `--plain`: emit plain tab-separated text for shell pipelines
- `--no-color`: disable ANSI color output

Examples:

```sh
aim list --json
aim update --check-only --csv
aim list --plain | grep obsidian
aim -C /tmp/aim-state add --dry-run ./Example.AppImage
aim update unset example-app --yes
```

## Output and exit status

`aim` keeps its process interface script-friendly:

- primary command output, including `--json`, `--csv`, and `--plain`, is written to stdout
- errors, warnings, prompts, progress, and verbose diagnostics are written to stderr
- success exits with `0`; failures exit with a stable non-zero code

For unexpected internal failures, `aim` prints a short bug-report path:

- a concise failure summary
- a hint to rerun with `--verbose`
- the GitHub issues URL for reporting the problem

Expected errors are rewritten to be user-facing and actionable when possible, for example by suggesting `aim list`, `aim update set`, or a writable `-C` state root.

Exit codes:

- `0`: success
- `64`: invalid command usage
- `66`: requested local input or resource not found
- `69`: external service or tool unavailable
- `70`: internal or uncategorized software failure
- `73`: local write, create, or update failure
- `75`: temporary or retryable runtime failure
- `77`: permission, confirmation, or precondition refusal

## Help and terminal docs

`aim` exposes two help surfaces:

- `aim` shows concise getting-started help
- `aim --help` and `aim <command> --help` show the full inline reference
- `aim help` and `aim help <command>` show terminal manual pages backed by the installed command tree

Examples:

```sh
aim
aim --help
aim help
aim help update
aim help update set
```

Manual pages are also generated for direct `man` use:

```sh
man aim
man aim-add
man aim-update-set
```

## Command Reference

### list

List managed AppImages in human-readable text, plain text, JSON, or CSV form.

```sh
aim list
aim list --json
aim list --csv
aim list --plain
```

### migrate

Repair managed state and migrate legacy paths.

```sh
aim migrate
aim migrate example-app
aim migrate --dry-run --json
```

### update set

Set the configured update source for a managed app.

```sh
aim update set example-app --github owner/repo
aim update set example-app --embedded
aim update set example-app --zsync https://example.com/Example.AppImage.zsync
```

### update unset

Unset the configured update source for a managed app.

```sh
aim update unset example-app
aim update unset example-app --dry-run --json
```

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
