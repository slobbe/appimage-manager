# AppImage Manager (aim)

![GitHub Release](https://img.shields.io/github/v/release/slobbe/appimage-manager?sort=semver&display_name=release&style=flat-square)
![GitHub License](https://img.shields.io/github/license/slobbe/appimage-manager?style=flat-square)

Manage AppImages as desktop apps on Linux. Install, integrate, update and remove AppImages from the terminal.

> [!WARNING]
> This project is still **work-in-progess**.
> Breaking changes may occur anytime while still in **v0.x.x**.

## Installation

Downloads the latest release binary and installs it to `~/.local/bin/aim`.

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

`aim` manages AppImages on Linux. It is intentionally scoped to:

- desktop integration and removal
- a simple local database of managed apps
- update checks and applies for managed apps using embedded zsync, GitHub releases, and GitLab releases
- self-upgrade via `aim upgrade`

### Upgrade `aim`

Upgrade `aim` to the latest stable release.

```sh
aim upgrade
```

### `aim add`: Add/Install an AppImage

Add a remote source, managed app, or local `.AppImage`.

```sh
aim add <https-url|github:owner/repo|gitlab:namespace/project|path-to.AppImage|id>
```

`aim add` is the umbrella command. It routes remote sources through the remote install flow and local paths or managed IDs through local integration.

Examples:

```sh
aim add ./MyApp.AppImage
aim add <id>
aim add https://example.com/MyApp.AppImage
aim add github:owner/repo
aim add gitlab:namespace/project
aim add github:owner/repo --asset "MyApp-*-x86_64.AppImage"
aim add https://example.com/MyApp.AppImage --sha256 <64-hex>
```

| Option     | Meaning                                                               |
| :--------- | :-------------------------------------------------------------------- |
| `--asset`  | asset filename pattern override for `github:`/`gitlab:` add sources   |
| `--sha256` | expected SHA-256 for direct `https://` add sources                    |

#### `aim integrate`: Integrate a local AppImage

Integrate a local `.AppImage` or reintegrate an existing managed ID explicitly.

```sh
aim integrate <path-to.AppImage|id>
```

Examples:

```sh
aim integrate ./MyApp.AppImage
aim integrate <id>
```

#### `aim install`: Install AppImage from a remote source

Download from a remote source and integrate the result. `aim add` is the umbrella/default path; `aim install` remains the explicit remote-only command.

```sh
aim install https://example.com/MyApp.AppImage
aim install https://example.com/MyApp.AppImage --sha256 <64-hex>
aim install github:owner/repo
aim install gitlab:namespace/project
aim install github:owner/repo --asset "MyApp-*-x86_64.AppImage"
```

| Option     | Meaning                                                                   |
| :--------- | :------------------------------------------------------------------------ |
| `--asset`  | asset filename pattern override for `github:`/`gitlab:` install sources   |
| `--sha256` | expected SHA-256 for direct `https://` install sources                    |

For direct `https://` installs, `--sha256` is optional. If omitted, `aim` warns that checksum verification is skipped for that download. Direct URL installs are one-off remote installs and persist `UpdateNone`.

For `github:` and `gitlab:` installs, `aim` configures the matching update source automatically.

### `aim info`: Get information and metadata about an AppImage

Inspect a package ref, managed app, or local `.AppImage` with one command. `aim info` automatically routes to `show` or `inspect` based on the input.

```sh
aim info github:owner/repo
aim info gitlab:namespace/project
aim info helium
aim info ./MyApp.AppImage
```

#### `aim inspect`: Inspect local AppImage metadata

Inspect a managed app or a local `.AppImage`. Use `aim info` if you want the same behavior behind a single umbrella command.

```sh
aim inspect <id|path-to.AppImage>
```

Examples:

```sh
aim inspect myapp
aim inspect ./MyApp.AppImage
```

#### `aim show`: Show remote metadata before installing

Inspect a package ref before installing it. Use `aim info` if you want a convenience command that also accepts managed app IDs and local AppImages.

```sh
aim show github:owner/repo
aim show gitlab:namespace/project
```

### `aim remove`: Remove AppImage

Remove a managed AppImage or unlink its desktop integration.

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

### `aim list`: List all managed AppImages

List managed AppImages.

```sh
aim list [options]
```

| Option               | Meaning                       |
| :------------------- | :---------------------------- |
| `--all`, `-a`        | list all AppImages (default)  |
| `--integrated`, `-i` | list integrated AppImages only |
| `--unlinked`, `-u`   | list only unlinked AppImages  |

Unlinked entries are still managed by `aim`, but currently have no desktop integration.

### `aim update`: Update AppImages

Check, apply, or configure updates for managed apps.

Check or apply updates:

```sh
aim update
aim update --check-only
aim update --yes
```

Check one managed app by ID:

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

Set an update source:

```sh
aim update set <id> --github owner/repo
aim update set <id> --gitlab namespace/project
aim update set <id> --zsync-url https://example.com/MyApp.AppImage.zsync
aim update set <id> --embedded
aim update unset <id>
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
| `--embedded`      | use the update source embedded in the current AppImage   |

GitHub and GitLab update checks use stable releases only.
When a GitHub or GitLab release asset also publishes a sibling `.zsync` file at the same URL plus `.zsync`, `aim update` automatically tries a delta update first.
If the sibling `.zsync` is missing or `zsync` cannot be used, `aim` falls back to downloading the full AppImage.
The configured update source remains GitHub or GitLab; `aim` only switches the transport used during update apply.

If an AppImage embeds zsync update info, local integration via `aim add` or `aim integrate` preserves it automatically.
For remote `aim add` and `aim install` commands, the selected remote source becomes the app's configured update source instead.
Use `aim inspect` to view the embedded source in a managed or local AppImage.
Use `aim update set <id> --embedded` to switch back to the embedded source later.
If the current AppImage does not embed an update source, `aim` tells you and, when another source is configured, offers to unset it or keep it.
Use `aim update unset <id>` to clear any configured update source explicitly.

## Where `aim` stores files

- App files: `${XDG_DATA_HOME:-~/.local/share}/aim`
- Desktop links: `${XDG_DATA_HOME:-~/.local/share}/applications`
- Desktop icons: `${XDG_DATA_HOME:-~/.local/share}/icons/hicolor` and `${XDG_DATA_HOME:-~/.local/share}/pixmaps`
- Config files: `${XDG_CONFIG_HOME:-~/.config}/aim`
- Database: `${XDG_STATE_HOME:-~/.local/state}/aim/apps.json`
- Temporary files: `${XDG_CACHE_HOME:-~/.cache}/aim/tmp`

`aim` uses XDG base directories. Legacy installs from `~/.appimage-manager` and older XDG paths under `appimage-manager` are migrated automatically on startup. When multiple legacy sources exist, `aim` uses the newest legacy `apps.json` to choose the preferred migration source and prefers files from that source for migrated data. The migration preserves the old directories.

## License

[MIT](/LICENSE)
