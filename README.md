# `aim` - AppImage Manager CLI

[![GitHub Release](https://img.shields.io/github/v/release/slobbe/appimage-manager?sort=semver&display_name=release&style=flat-square&color=royalblue)](https://github.com/slobbe/appimage-manager/releases/latest)
[![GitHub License](https://img.shields.io/github/license/slobbe/appimage-manager?style=flat-square&color=teal)](/LICENSE)

A simple CLI to install, integrate, and update AppImages.

> [!NOTE]
> This project is still a **work in progress** and breaking changes may happen at any time while in `v0.x.x`.

## Install

```sh
curl -fsSL https://raw.githubusercontent.com/slobbe/appimage-manager/main/scripts/install.sh | sh
aim --version
```

To install a pinned version:

```sh
curl -fsSL https://raw.githubusercontent.com/slobbe/appimage-manager/main/scripts/install.sh | AIM_VERSION=v0.15.4 sh
```

If `aim` is not found, make sure `~/.local/bin` is on your `PATH`.

To uninstall the files installed by the installer:

```sh
curl -fsSL https://raw.githubusercontent.com/slobbe/appimage-manager/main/scripts/uninstall.sh | sh
```

## How to Use

### Add an AppImage

```sh
aim add ./Example.AppImage
aim add --url https://example.com/Example.AppImage
aim add --github owner/repo
```

### Check and apply updates

```sh
aim update
aim update --check-only
```

### Set or clear an update source

```sh
aim update --set example-app --github owner/repo
aim update --set example-app --zsync https://example.com/Example.AppImage.zsync
aim update --set example-app --embedded
aim update --unset example-app
```

### Remove an AppImage

```sh
aim remove example-app
aim remove --link example-app
```

## Useful Commands

```sh
aim list                 # list managed AppImages
aim info example-app     # inspect a managed app
aim info ./Example.AppImage
aim info --github owner/repo
aim --upgrade            # upgrade aim itself
```

## Key Flags

- `-n`, `--dry-run`: preview changes without applying them
- `-y`, `--yes`: skip confirmation prompts
- `--no-input`: disable interactive prompting
- `--json`: emit machine-readable JSON where supported
- `-q`, `--quiet`: reduce non-essential status output
- `-d`, `--debug`: enable diagnostic logs

## Storage

`aim` uses XDG base directories:

- AppImage files: `${XDG_DATA_HOME:-~/.local/share}/aim`
- Desktop links: `${XDG_DATA_HOME:-~/.local/share}/applications`
- Desktop icons: `${XDG_DATA_HOME:-~/.local/share}/icons/hicolor`
- Config files: `${XDG_CONFIG_HOME:-~/.config}/aim`
- Database: `${XDG_STATE_HOME:-~/.local/state}/aim/apps.json`
- Temporary files: `${XDG_CACHE_HOME:-~/.cache}/aim/tmp`

## More Help

- `aim --help` for the CLI overview
- `aim help <command>` for command-specific manual pages
- `aim <command> --help` for flags and usage on a specific command

## License

[MIT](/LICENSE)
