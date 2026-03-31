# AppImage Manager (aim)

[![GitHub Release](https://img.shields.io/github/v/release/slobbe/appimage-manager?sort=semver&display_name=release&style=flat-square&color=royalblue)](https://github.com/slobbe/appimage-manager/releases/latest)
[![GitHub License](https://img.shields.io/github/license/slobbe/appimage-manager?style=flat-square&color=teal)](/LICENSE)

AppImage Manager helps you manage AppImages from the terminal without manual desktop setup or update juggling. It handles install, integration, updates, and removal in one place. The goal is to make AppImage integration seamless and as close to a common package manager as possible.

> [!NOTE]
> This project is still a work in progress.
> Breaking changes may occur at any time while it remains in **v0.x.x**.

## Features

- Install AppImages from local files, direct URLs, GitHub, and GitLab
- Integrate AppImages with desktop menus, icons, and launchers
- Track managed apps and update them from configured sources
- Run explicit migration and desktop integration repair when needed
- Remove apps or unlink their desktop integration
- Inspect AppImage metadata and update-source details

## Installation

```sh
# Download and install
curl -fsSL https://raw.githubusercontent.com/slobbe/appimage-manager/main/scripts/install.sh | sh

# Verify
aim --version
```

If `aim` is not found, make sure `~/.local/bin` is on your `PATH`.

## Build from source

```sh
git clone https://github.com/slobbe/appimage-manager.git
cd appimage-manager
go build ./cmd/aim
```

Requirements: Linux, Go 1.25.5+.

To regenerate the committed man page from the CLI definition:

```sh
go run -tags docgen ./cmd/aim
```

The committed man page is generated from the Cobra command tree with the development version string (`dev`). GitHub Actions release builds inject the release version and assemble the release tarballs. `scripts/build.sh` mirrors that packaging flow locally for reproducible debugging.

Official releases are published by GitHub Actions when a tag matching `vX.Y.Z` is pushed. The Git tag keeps the `v` prefix, while the release title and assets drop it, for example `v0.12.5` -> release title `0.12.5` and assets `aim-0.12.5-linux-amd64.tar.gz` and `aim-0.12.5-linux-arm64.tar.gz`.

To reproduce the same release artifacts locally:

```sh
scripts/build.sh v0.12.5
```

## Usage

### Upgrade `aim`

Upgrade `aim` to the latest stable release with the official installer. This also refreshes the man page and shell completions.

Interactive terminals show progress while `aim` checks for updates and runs the installer.

```sh
aim --upgrade
```

### `aim add`: Add/Install an AppImage

Add a remote source, a managed app, or a local `.AppImage`.

```sh
aim add [<https-url|github-url|gitlab-url|Path/To.AppImage|id>]
aim add --github owner/repo
aim add --gitlab namespace/project
```

`aim add` handles remote sources, managed IDs, and local AppImages. Use `--github`, `--gitlab`, or a recognized provider URL for provider-based sources.

Interactive terminals show progress while `aim` resolves provider metadata, inspects AppImages, and integrates installed files.

Examples:

```sh
aim add ./MyApp.AppImage
aim add <id>
aim add https://example.com/MyApp.AppImage
aim add --github owner/repo
aim add --gitlab namespace/project
aim add https://github.com/owner/repo
aim add --github owner/repo --asset "MyApp-*-x86_64.AppImage"
aim add https://example.com/MyApp.AppImage --sha256 <64-hex>
```

| Option     | Meaning                                            |
| :--------- | :------------------------------------------------- |
| `--github` | GitHub repo in the form `owner/repo`               |
| `--gitlab` | GitLab project path `namespace/project`            |
| `--asset`  | asset filename pattern override for provider adds  |
| `--sha256` | expected SHA-256 for direct `https://` add sources |

For direct `https://` downloads, `--sha256` is optional. If omitted, `aim` warns that checksum verification is skipped. Direct URL adds are one-off installs and remain `UpdateNone`. Provider adds configure the matching update source automatically.

### `aim info`: Get information and metadata about an AppImage

Inspect a provider package, a managed app, or a local `.AppImage`.

```sh
aim info [<github-url|gitlab-url|id|Path/To.AppImage>]
aim info --github owner/repo
aim info --gitlab namespace/project
aim info https://github.com/owner/repo
aim info helium
aim info ./MyApp.AppImage
```

`aim info` accepts recognized GitHub/GitLab repo and project URLs, but it does not accept arbitrary `https://` download URLs.

Interactive terminals show progress while `aim` resolves provider metadata or inspects a local AppImage.

### `aim migrate`: Run migration and desktop integration repair

Run legacy path migration, desktop integration repair, and deep managed AppImage reconciliation explicitly.

```sh
aim migrate
aim migrate <id>
aim repair
aim repair <id>
```

`aim migrate` may inspect AppImages and can take noticeably longer than ordinary commands. Normal commands like `aim list` and `aim info` do not automatically run migration or repair work.
Interactive terminals show progress while migration and repair are running.

### `aim remove`: Remove AppImage

Remove a managed AppImage or unlink it from the desktop.

```sh
aim remove [--unlink] <id>
```

| Option         | Meaning                                                  |
| :------------- | :------------------------------------------------------- |
| `--unlink`     | remove only desktop integration; keep `.AppImage` and its data |

### `aim list`: List all managed AppImages

```sh
aim list [options]
```

| Option               | Meaning                       |
| :------------------- | :---------------------------- |
| `--all`, `-a`        | list all AppImages (default)  |
| `--integrated`, `-i` | list integrated AppImages only |
| `--unlinked`, `-u`   | list only unlinked AppImages  |

Unlinked entries are still managed, but currently have no desktop integration.

### `aim update`: Update AppImages

```sh
aim update
aim update --check-only
aim update <id>
aim update --yes
```

| Option               | Meaning                            |
| :------------------- | :--------------------------------- |
| `--yes`, `-y`        | apply found updates without prompt |
| `--check-only`, `-c` | check only; do not apply           |

Set an update source:

```sh
aim update set <id> --github owner/repo
aim update set <id> --gitlab namespace/project
aim update set <id> --zsync https://example.com/MyApp.AppImage.zsync
aim update set <id> --embedded
aim update unset <id>
```

For GitHub and GitLab sources, `--asset` is optional and defaults to `*.AppImage`. With that default, `aim` prefers the AppImage that matches the current machine architecture when multiple AppImage assets are present. Use `--asset` only if you need a narrower match:

```sh
aim update set <id> --github owner/repo --asset "MyApp-*-x86_64.AppImage"
```

| Option            | Meaning                                                  |
| :---------------- | :------------------------------------------------------- |
| `--github`        | GitHub repo in the form owner/repo                       |
| `--gitlab`        | GitLab project path in the form namespace/project        |
| `--asset`         | asset filename pattern; defaults to `*.AppImage`         |
| `--zsync`         | direct zsync metadata URL (HTTPS)                        |
| `--embedded`      | use the update source embedded in the current AppImage   |

GitHub and GitLab update checks use stable releases only. If a matching release asset also has a sibling `.zsync` file, `aim update` tries a delta update first and falls back to a full download if needed.

If an AppImage embeds zsync update info, local `aim add` preserves it automatically. Use `aim info` to inspect the embedded source, `aim update set <id> --embedded` to switch to it, and `aim update unset <id>` to clear any configured source.

## Where `aim` stores files

`aim` uses XDG base directories.

- App files: `${XDG_DATA_HOME:-~/.local/share}/aim`
- Desktop links: `${XDG_DATA_HOME:-~/.local/share}/applications`
- Desktop icons: `${XDG_DATA_HOME:-~/.local/share}/icons/hicolor` and `${XDG_DATA_HOME:-~/.local/share}/pixmaps`
- Config files: `${XDG_CONFIG_HOME:-~/.config}/aim`
- Database: `${XDG_STATE_HOME:-~/.local/state}/aim/apps.json`
- Temporary files: `${XDG_CACHE_HOME:-~/.cache}/aim/tmp`

## License

[MIT](/LICENSE)
