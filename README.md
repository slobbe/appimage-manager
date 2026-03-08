# AppImage Manager (aim)

![GitHub Release](https://img.shields.io/github/v/release/slobbe/appimage-manager?sort=semver&display_name=release&style=flat-square)
![GitHub License](https://img.shields.io/github/license/slobbe/appimage-manager?style=flat-square)

Integrate AppImages into your desktop and keep them updated through embedded zsync metadata or explicit GitHub/GitLab release sources.

## Quick start

```sh
aim add ./MyApp.AppImage
aim list
aim update --check-only
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

`aim` manages local AppImages on Linux. It is intentionally scoped to:

- desktop integration and removal
- a simple local database of managed apps
- update checks and applies for embedded zsync, GitHub releases, and GitLab releases
- self-upgrade via `aim upgrade`

**Upgrade** `aim` itself to the latest stable release:

```sh
aim upgrade
```

**Integrate** AppImage into your desktop environment:

```sh
aim add <path-to.AppImage|id>
```

If given an ID of an unlinked AppImage it reintegrates it.

**Remove** AppImage:

```sh
aim remove [--unlink] <id>
```

| Option         | Meaning                                                  |
| :------------- | :------------------------------------------------------- |
| `--unlink`     | remove only desktop integration; keep managed AppImage files |

Without `--unlink`, `aim remove` removes the managed app entry and its managed files.

Example:

```sh
aim remove --unlink <id>
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
aim update --check-only
aim update --yes
```

**Check** one installed app by ID (and optionally apply):

```sh
aim update <id>
aim update <id> --check-only
```

| Option               | Meaning                            |
| :------------------- | :--------------------------------- |
| `--yes`, `-y`        | apply found updates without prompt |
| `--check-only`, `-c` | check only; do not apply           |

**Check** a local AppImage file update-info:

```sh
aim update check <path-to.AppImage>
```

This only supports AppImages that expose embedded zsync update information.

**Set** update source for an AppImage:

```sh
aim update set <id> --github owner/repo
aim update set <id> --gitlab namespace/project
aim update set <id> --zsync-url https://example.com/MyApp.AppImage.zsync
```

For GitHub and GitLab sources, `--asset` is optional and defaults to `*.AppImage`.
Use `--asset` only if you need a narrower asset match, for example:

```sh
aim update set <id> --github owner/repo --asset "MyApp-*-x86_64.AppImage"
```

| Option            | Meaning                                                  |
| :---------------- | :------------------------------------------------------- |
| `--github`        | GitHub repo in the form owner/repo                       |
| `--gitlab`        | GitLab project path in the form namespace/project        |
| `--asset`         | asset filename pattern; defaults to `*.AppImage`         |
| `--zsync-url`     | direct zsync metadata URL (HTTPS)                        |

GitHub and GitLab update checks use stable releases only.

If an AppImage embeds zsync update info, `aim add` preserves it automatically.

Removed from scope:

- manifest-based update sources
- direct URL update sources
- pin/unpin commands
- self-upgrade via `aim --upgrade`

If an older database entry still references an unsupported update source, `aim update` will tell you to reconfigure it with `aim update set`.

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
