# AppImage Manager (aim)

![GitHub Release](https://img.shields.io/github/v/release/slobbe/appimage-manager?sort=semver&display_name=release&style=flat-square)
![GitHub License](https://img.shields.io/github/license/slobbe/appimage-manager?style=flat-square)

Manage AppImages as desktop apps on Linux.

## Quick start

```sh
aim add ./MyApp.AppImage
aim list
```

## Installation

Downloads the latest release for your CPU (amd64/x86_64 or arm64/aarch64) and installs it to `~/.local/bin/aim`.

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

## Usage

Integration installs desktop entry metadata and icons so the AppImage appears in your launcher.

`aim` manages local AppImages on Linux. It is intentionally scoped to:

- desktop integration and removal
- a simple local database of managed apps
- update checks and applies for managed apps using embedded zsync, GitHub releases, and GitLab releases
- self-upgrade via `aim upgrade`

**Upgrade** `aim` to the latest stable release:

```sh
aim upgrade
```

**Integrate** an AppImage, download from a remote source, or reintegrate an existing ID:

```sh
aim add <path-to.AppImage|id|https-url|github:owner/repo|gitlab:namespace/project>
```

If the argument is the ID of an unlinked app, `aim` reintegrates it.
If the argument is `github:owner/repo` or `gitlab:namespace/project`, `aim` downloads the latest stable matching AppImage and configures updates from that source automatically.

Examples:

```sh
aim add https://example.com/MyApp.AppImage
aim add https://example.com/MyApp.AppImage --sha256 <64-hex>
aim add github:owner/repo
aim add gitlab:namespace/project
aim add github:owner/repo --asset "MyApp-*-x86_64.AppImage"
```

| Option         | Meaning                                                                   |
| :------------- | :------------------------------------------------------------------------ |
| `--asset`      | asset filename pattern; defaults to `*.AppImage` for `github:`/`gitlab:` |
| `--sha256`     | expected SHA-256 for direct `https://` add sources                        |
| `--post-check` | run post-integration update check for zsync-enabled apps                  |

For direct `https://` adds, `--sha256` is optional. If omitted, `aim` warns that checksum verification is skipped for that download.

**Remove** or unlink a managed AppImage:

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

**List** managed AppImages:

```sh
aim list [options]
```

| Option               | Meaning                       |
| :------------------- | :---------------------------- |
| `--all`, `-a`        | list all AppImages (default)  |
| `--integrated`, `-i` | list integrated AppImages only |
| `--unlinked`, `-u`   | list only unlinked AppImages  |

Unlinked entries are still managed by `aim`, but currently have no desktop integration.

**Check** all managed apps for updates:

```sh
aim update
aim update --check-only
aim update --yes
```

**Check** one managed app by ID:

```sh
aim update <id>
aim update <id> --check-only
```

Normal update flow:

```sh
# check all managed apps
aim update

# check one managed app
aim update <id>

# apply available updates without prompting
aim update --yes
```

| Option               | Meaning                            |
| :------------------- | :--------------------------------- |
| `--yes`, `-y`        | apply found updates without prompt |
| `--check-only`, `-c` | check only; do not apply           |

**Set** update source for an AppImage:

```sh
aim update set <id> --github owner/repo
aim update set <id> --gitlab namespace/project
aim update set <id> --zsync-url https://example.com/MyApp.AppImage.zsync
```

For GitHub and GitLab sources, `--asset` is optional and defaults to `*.AppImage`.
With that default, `aim` still prefers the AppImage that matches the current machine architecture when multiple AppImage assets are present.
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
For `github:` and `gitlab:` adds, the selected remote source becomes the app's configured update source instead.

## Where `aim` stores files

- App files: `${XDG_DATA_HOME:-~/.local/share}/appimage-manager`
- Desktop links: `${XDG_DATA_HOME:-~/.local/share}/applications`
- Desktop icons: `${XDG_DATA_HOME:-~/.local/share}/icons/hicolor` and `${XDG_DATA_HOME:-~/.local/share}/pixmaps`
- Database: `${XDG_STATE_HOME:-~/.local/state}/appimage-manager/apps.json`
- Temporary files: `${XDG_CACHE_HOME:-~/.cache}/appimage-manager/tmp`

`aim` uses XDG base directories. Legacy installs from `~/.appimage-manager` are migrated automatically on startup.

## Notes

- AppImages are detected by the `.AppImage` extension.

## License

[MIT](/LICENSE)
