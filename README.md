# AppImage Manager (aim)

![GitHub Release](https://img.shields.io/github/v/release/slobbe/appimage-manager?sort=semver&display_name=release&style=flat-square)
![GitHub License](https://img.shields.io/github/license/slobbe/appimage-manager?style=flat-square)

Manage AppImages from the command line. Add, inspect, update, and remove AppImages on Linux.

> [!WARNING]
> This project is still **work-in-progess**.
> Breaking changes may occur anytime while still in **v0.x.x**.

## Installation

`scripts/install.sh` installs the latest release to `~/.local/bin/aim`, the man page to `${XDG_DATA_HOME:-$HOME/.local/share}/man/man1/aim.1`, and shell completions under `${XDG_DATA_HOME:-$HOME/.local/share}`.

```sh
# Download and install
curl -fsSL https://raw.githubusercontent.com/slobbe/appimage-manager/main/scripts/install.sh | sh

# Verify
aim --version
man aim
```

If `aim` is not found, make sure `~/.local/bin` is on your `PATH`. If `man aim` is not found, make sure your local man path includes `${XDG_DATA_HOME:-$HOME/.local/share}/man`. Start a new shell session if completions do not appear immediately.

Completion files are installed to:

- Bash: `${XDG_DATA_HOME:-$HOME/.local/share}/bash-completion/completions/aim`
- Zsh: `${XDG_DATA_HOME:-$HOME/.local/share}/zsh/site-functions/_aim`
- Fish: `${XDG_DATA_HOME:-$HOME/.local/share}/fish/vendor_completions.d/aim.fish`

On Zsh, you may need to ensure `${XDG_DATA_HOME:-$HOME/.local/share}/zsh/site-functions` is present in `fpath`.

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

The committed man page is generated from the Cobra command tree with the development version string (`dev`). Release builds inject the release version via `scripts/build.sh`.

## Usage

`aim` is intentionally scoped to:

- desktop integration and removal
- a small local database of managed apps
- update checks and apply for managed apps
- self-upgrade via `aim upgrade`

### Upgrade `aim`

Upgrade `aim` to the latest stable release.

```sh
aim upgrade
```

### `aim add`: Add/Install an AppImage

Add a remote source, managed app, or local `.AppImage`.

```sh
aim add [<https-url|github-url|gitlab-url|Path/To.AppImage|id>]
aim add --github owner/repo
aim add --gitlab namespace/project
```

`aim add` handles remote sources, managed IDs, and local AppImages. Use `--github`, `--gitlab`, or a recognized provider URL for provider sources.

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

For direct `https://` downloads, `--sha256` is optional. If omitted, `aim` warns that checksum verification is skipped. Direct URL adds are one-off installs and persist `UpdateNone`. Provider adds configure the matching update source automatically.

### `aim info`: Get information and metadata about an AppImage

Inspect a provider package, managed app, or local `.AppImage`.

```sh
aim info [<github-url|gitlab-url|id|Path/To.AppImage>]
aim info --github owner/repo
aim info --gitlab namespace/project
aim info https://github.com/owner/repo
aim info helium
aim info ./MyApp.AppImage
```

`aim info` accepts recognized GitHub/GitLab repo and project URLs, but it does not accept arbitrary `https://` download URLs.

### `aim remove`: Remove AppImage

Remove a managed AppImage or unlink its desktop integration.

```sh
aim remove [--unlink] <id>
```

| Option         | Meaning                                                  |
| :------------- | :------------------------------------------------------- |
| `--unlink`     | remove only desktop integration; keep managed AppImage files |

Without `--unlink`, `aim remove` also removes the managed app entry and files.

```sh
aim remove --unlink <id>
```

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

- App files: `${XDG_DATA_HOME:-~/.local/share}/aim`
- Desktop links: `${XDG_DATA_HOME:-~/.local/share}/applications`
- Desktop icons: `${XDG_DATA_HOME:-~/.local/share}/icons/hicolor` and `${XDG_DATA_HOME:-~/.local/share}/pixmaps`
- Config files: `${XDG_CONFIG_HOME:-~/.config}/aim`
- Database: `${XDG_STATE_HOME:-~/.local/state}/aim/apps.json`
- Temporary files: `${XDG_CACHE_HOME:-~/.cache}/aim/tmp`

`aim` uses XDG base directories.

## License

[MIT](/LICENSE)
