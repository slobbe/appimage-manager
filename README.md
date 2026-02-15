# AppImage Manager (aim)

Integrate, remove, and update AppImages with one command.

A lightweight CLI that registers AppImages with your desktop so they behave like installed apps.

## Quick start

```sh
aim add ./MyApp.AppImage
aim list
```

## Installation

Downloads the latest release for your CPU (amd64/x86_64 or arm64/aarch64) and installs it to `~/.local/bin/aim`.

```sh
curl -fsSL https://raw.githubusercontent.com/slobbe/appimage-manager/main/scripts/install.sh | sh
```

Verify:
```sh
aim --version
```

If `aim` is not found, make sure `~/.local/bin` is on your `PATH`.

## Build from source

Requirements: Linux, Go 1.25.5+.

```sh
git clone https://github.com/slobbe/appimage-manager.git
cd appimage-manager
```

```sh
go build ./cmd/aim
```

## Usage

Integration creates desktop entry metadata (and icon data when available) so the AppImage appears in your launcher.

Global flags:

| Option       | Meaning                    |
| :----------- | :------------------------- |
| `--no-color` | disable ANSI color output  |
| `--upgrade`  | self-update to latest stable release |

**Upgrade** `aim` itself to the latest stable release:

```sh
aim --upgrade
```

**Integrate** AppImage into your desktop environment:

```sh
aim add <path-to.AppImage|id>
```

If given an ID of an unlinked AppImage it reintegrates it.

**Remove** AppImage:

```sh
aim remove [options] <id>
```

| Option         | Meaning                                                  |
| :------------- | :------------------------------------------------------- |
| `--keep`, `-k` | keep the AppImage file; remove only desktop integration  |

Example:

```sh
aim remove --keep <id>
```

**List** all integrated AppImages:

```sh
aim list [options]
```

| Option               | Meaning                       |
| :------------------- | :---------------------------- |
| `--all`, `-a`        | list all AppImages (default)  |
| `--integrated`, `-i` | list integrated AppImages only |
| `--unlinked`, `-u`   | list only unlinked AppImages  |

Unlinked entries are AppImages known to the database without a current desktop integration.

**Check** all apps for updates (and optionally apply):

```sh
aim update
```

**Check** one installed app by ID (and optionally apply):

```sh
aim update <id>
```

| Option               | Meaning                            |
| :------------------- | :--------------------------------- |
| `--yes`, `-y`        | apply found updates without prompt |
| `--check-only`, `-c` | check only; do not apply           |

**Check** a local AppImage file update-info:

```sh
aim update check <path-to.AppImage>
```

**Set** update source for an AppImage:

```sh
aim update set <id> --github owner/repo --asset "*.AppImage"
aim update set <id> --gitlab namespace/project --asset "*.AppImage"
aim update set <id> --zsync-url https://example.com/MyApp.AppImage.zsync
aim update set <id> --manifest-url https://example.com/myapp/latest.json
aim update set <id> --url https://example.com/MyApp.AppImage --sha256 <sha256>
```

| Option            | Meaning                                                  |
| :---------------- | :------------------------------------------------------- |
| `--github`        | GitHub repo in the form owner/repo                       |
| `--gitlab`        | GitLab project path in the form namespace/project        |
| `--asset`         | asset filename pattern, e.g. `MyApp-*.AppImage`          |
| `--zsync-url`     | direct zsync metadata URL (HTTPS)                        |
| `--manifest-url`  | manifest endpoint URL (HTTPS)                            |
| `--url`           | direct AppImage URL (HTTPS)                              |
| `--sha256`        | required with `--url`; expected SHA-256 for verification |

GitHub and GitLab update checks use stable releases only.

**Pin** an app to skip it during batch update apply:

```sh
aim pin <id>
```

**Unpin** an app so batch update apply can include it again:

```sh
aim unpin <id>
```

## Data locations (XDG)

`aim` stores files using XDG base directories:

- App files: `${XDG_DATA_HOME:-~/.local/share}/appimage-manager`
- Desktop links: `${XDG_DATA_HOME:-~/.local/share}/applications`
- Desktop icons: `${XDG_DATA_HOME:-~/.local/share}/icons/hicolor` and `${XDG_DATA_HOME:-~/.local/share}/pixmaps`
- Database: `${XDG_STATE_HOME:-~/.local/state}/appimage-manager/apps.json`
- Temporary files: `${XDG_CACHE_HOME:-~/.cache}/appimage-manager/tmp`

`aim` currently does not persist a user config file, so `${XDG_CONFIG_HOME}` is reserved for future configuration support.

Legacy installs from `~/.appimage-manager` are migrated automatically on startup.

## Notes

- AppImages are detected by the `.AppImage` extension.

## License

[MIT](/LICENSE)
