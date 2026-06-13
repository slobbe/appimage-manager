# AppImage Manager (`aim`)

A CLI to install, integrate, inspect, and update AppImages on Linux.

[![](https://shieldcn.dev/group/github/release/slobbe/appimage-manager+github/license/slobbe/appimage-manager.svg?variant=secondary&size=xs)](https://github.com/slobbe/appimage-manager/releases/latest)

> [!NOTE]
> This project is still a **work in progress** and breaking changes may happen at any time while in `v0.x.x`.

## Install

Install the latest release:

```sh
curl -fsSL https://raw.githubusercontent.com/slobbe/appimage-manager/main/scripts/install.sh | sh
aim --version
```

Install a specific version:

```sh
curl -fsSL https://raw.githubusercontent.com/slobbe/appimage-manager/main/scripts/install.sh | AIM_VERSION=v0.17.0 sh
```

`AIM_VERSION` accepts either `0.17.0`, `v0.17.0`, or a prerelease tag such as `v0.17.0-rc.1`.

The installer places `aim` in `~/.local/bin` by default and generates man pages and shell completions locally from the installed binary.

If `aim` is not found, make sure `~/.local/bin` is on your `PATH`.

## Common commands

### Add an AppImage

```sh
aim add ./Example.AppImage
aim add --github owner/repo
aim add --github owner/repo --prerelease
```

### Check and apply updates

```sh
aim update
aim update example-app
```

### Set or clear an update source

```sh
aim update --set example-app --github owner/repo
aim update --set example-app --github owner/repo --prerelease
aim update --set example-app --embedded
aim update --unset example-app
```

### Remove an AppImage

```sh
aim remove example-app
```

### Inspect, list, and locate data

```sh
aim info example-app
aim info ./Example.AppImage
aim list
aim paths
```

`aim info <path>` inspects a local AppImage before integration. Inspection executes the AppImage's extraction/update-info modes to read metadata; inspect only AppImages you trust.

### Update aim itself

```sh
aim selfupdate
aim selfupdate --prerelease
```

## Useful commands and aliases

```sh
aim list      # list managed AppImages
aim remove    # remove a managed AppImage
aim update    # check for and apply app updates
aim info      # inspect an AppImage or integrated app
aim paths     # show aim's config/storage/cache paths
```

## Global flags

- `--json`: emit machine-readable JSON where supported
- `--version`: print the current aim version

## More help

- `aim --help` for the CLI overview
- `aim help <command>` for command-specific manual pages
- `aim <command> --help` for flags and usage on a specific command
- `man aim` for the full manual page

## License

[MIT](/LICENSE)
