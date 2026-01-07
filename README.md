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

Requirements: Linux, Go 1.20+.

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

**Check** AppImage for updates:

```sh
aim update check <id>
```

Update checks currently work by ID. Local file checks are planned. GitHub release checks can target the latest release or pre-releases when enabled.

**Set** update source for an AppImage:

```sh
aim update set <id> --github owner/repo --asset "*.AppImage"
```

| Option           | Meaning                                              |
| :--------------- | :--------------------------------------------------- |
| `--github`       | GitHub repo in the form owner/repo                   |
| `--asset`        | asset filename pattern, e.g. `MyApp-*.AppImage`       |
| `--pre-release`  | allow pre-releases when checking for updates         |

## Notes

- AppImages are detected by the `.AppImage` extension.

## License

[MIT](/LICENSE)
