# AppImage Manager (`aim`)

A CLI to install, integrate, inspect, and update AppImages on Linux.

[![ci status](https://github.com/slobbe/appimage-manager/actions/workflows/ci.yml/badge.svg)](https://github.com/slobbe/appimage-manager/actions/workflows/ci.yml) [![release](https://img.shields.io/github/v/release/slobbe/appimage-manager)](https://github.com/slobbe/appimage-manager/releases) [![go version](https://img.shields.io/github/go-mod/go-version/slobbe/appimage-manager)](go.mod) [![license](https://img.shields.io/github/license/slobbe/appimage-manager)](LICENSE)

> [!NOTE]
> `aim` is still in **beta**. Expect occasional bugs and breaking changes. Feedback is welcome.

## Install

Install the latest release using the [install script](scripts/install.sh):

```sh
curl -fsSL https://raw.githubusercontent.com/slobbe/appimage-manager/main/scripts/install.sh | sh
```

<details>
<summary>Install a specific version</summary>

```sh
curl -fsSL https://raw.githubusercontent.com/slobbe/appimage-manager/main/scripts/install.sh \
  | AIM_VERSION=0.18.0 sh
```

`AIM_VERSION` accepts either a stable version like `0.18.0` or a prerelease version such as `v0.19.0-dev.1`.
</details>

The installer places `aim` in `~/.local/bin` by default and generates man pages and shell completions locally from the installed binary.

If `aim` is not found, make sure `~/.local/bin` is on your `PATH`.

## Common commands

### Add an AppImage

```sh
# add from a local file
aim add ./Example.AppImage

# add from a GitHub release
aim add --github owner/repo

# add a prerelease version from a GitHub release
aim add --github owner/repo --prerelease
```

Use `--asset`, `--exclude`, and `--prefer` to select more precisely among GitHub release assets. 

### Check and apply updates

```sh
# update all installed AppImages
aim update

# update a specific AppImage
aim update example-app
```

### Set or clear an update source

```sh
# set a GitHub update source
aim update --set example-app --github owner/repo

# set a prerelease version from a GitHub release
aim update --set example-app --github owner/repo --prerelease

# set embedded update source
aim update --set example-app --embedded

# unset the update source for a specific AppImage
aim update --unset example-app
```

Use `--asset`, `--exclude`, and `--prefer` to select more precisely among GitHub release assets. 

### Remove an AppImage

```sh
aim remove example-app
```

### Update aim itself

```sh
aim selfupdate
aim selfupdate --prerelease
```

### Other useful commands and aliases

```sh
aim list                # list all managed AppImages
aim info example-app    # inspect an AppImage or integrated app
aim paths               # show aim's config/storage/cache paths
aim diagnose            # check host FUSE compatibility
aim --help              # show help for aim
man aim                 # show man page for aim
```

### Global flags

- `--yes`: automatically confirm mutation prompts
- `--non-interactive`: fail instead of prompting for input
- `--color=auto|always|never`: control color in human-readable output

## Configuration

Run `aim paths` to print the effective paths for the current environment. By default, aim uses:

- `~/.config/aim/config.toml` for configuration
- `~/.local/share/aim/apps.json` for managed-app state
- `~/.local/share/aim/appimages` for installed AppImages
- `~/.local/share/applications` for desktop entries
- `~/.local/share/icons` for icons

The locations follow `XDG_CONFIG_HOME` and `XDG_DATA_HOME` when those variables are set. The currently supported configuration setting is:

```toml
appimage_dir = "~/Applications"
```

`appimage_dir` changes where managed AppImages are installed. Desktop entries, icons, and state continue to use their XDG data locations.

## License

[MIT](/LICENSE)
