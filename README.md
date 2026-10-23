# AppImage Manager (`aim`)

A CLI to install, integrate, inspect, and update AppImages on Linux.

![](https://shieldcn.dev/group/github/release/slobbe/appimage-manager+github/license/slobbe/appimage-manager.svg?variant=branded&size=sm)

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
curl -fsSL https://raw.githubusercontent.com/slobbe/appimage-manager/main/scripts/install.sh | AIM_VERSION=v0.18.0 sh
```

`AIM_VERSION` accepts either `0.18.0`, `v0.18.0`, or a prerelease tag such as `v0.18.0-rc.1`.

The installer places `aim` in `~/.local/bin` by default and generates man pages and shell completions locally from the installed binary.

If `aim` is not found, make sure `~/.local/bin` is on your `PATH`.

## Common commands

### Add an AppImage

```sh
aim add ./Example.AppImage
aim add --github owner/repo
aim add --github owner/repo --asset '*x86_64.AppImage'
aim add --github owner/repo --prerelease
aim add --github owner/repo --exclude '*debug*' --exclude '*minimal*' --prefer '*portable*'
```

### Check and apply updates

```sh
aim update --check
aim --json update --check
aim update
aim update example-app
```

`aim update --check` reports available updates without modifying installed AppImages. Today, `aim update` only checks and applies GitHub release update sources; embedded `zsync`, `local_file`, and unsupported update metadata is preserved for inspection but not applied yet.

### Set or clear an update source

```sh
aim update --set example-app --github owner/repo
aim update --set example-app --github owner/repo --asset '*x86_64.AppImage'
aim update --set example-app --github owner/repo --prerelease
aim update --set example-app --github owner/repo --exclude '*debug*' --prefer '*portable*' --prefer '*full*'
aim update --set example-app --embedded
aim update --unset example-app
```

Use `--asset`, `--exclude`, and `--prefer` to select among GitHub release assets, such as different architectures or flavors. All three use Go `filepath.Match` syntax, not regular expressions: `*` matches any sequence of non-separator characters, `?` matches one non-separator character, and `[abc]` or `[a-z]` matches a character class (`[^abc]` negates it). Patterns match the entire asset name and are case-sensitive. Quote patterns to prevent shell expansion.

`--exclude` and `--prefer` are repeatable and require `--github`, but do not require `--asset`. For updates, use them with `--set --github`; they cannot be combined with `--check`, `--unset`, or `--embedded`. Each occurrence is one pattern; commas are literal, not pattern separators. Empty or whitespace-only patterns are rejected.

Asset selection proceeds in this order:

1. Keep AppImage assets.
2. Apply `--asset`, if supplied.
3. Remove assets matching **any** `--exclude` pattern (OR).
4. Only when `--asset` is absent, keep the assets with the highest automatic selection score.
5. Keep assets matching **any** `--prefer` pattern (OR). If none match, retain the candidates from the previous step.
6. Require exactly one remaining asset; otherwise selection fails.

Exclusion is strict; preference is a soft tie-breaker and cannot restore excluded or lower-scoring assets. These patterns are saved with the GitHub update source for future updates. `aim info` shows them in human-readable output and as `asset_exclude_patterns` / `asset_prefer_patterns` arrays under `update_source` in JSON (omitted when empty).

`--embedded` preserves update metadata found inside the AppImage, but only embedded GitHub release sources are applied by `aim update` today.

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

### Diagnose AppImage compatibility

```sh
aim diagnose
aim --json diagnose
```

`aim diagnose` performs read-only host checks for the native-architecture FUSE 2 compatibility library in the linker cache and for `/dev/fuse` device access. FUSE 3 alone does not provide `libfuse.so.2`, which some AppImages require.
See [AppImage's FUSE troubleshooting documentation](https://docs.appimage.org/user-guide/troubleshooting/fuse.html) for further guidance.

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
aim diagnose  # check host FUSE compatibility without launching applications
```

## Global flags

- `--json`: emit machine-readable JSON where supported; this does not confirm
  mutations
- `--yes`: automatically confirm mutation prompts
- `--non-interactive`: fail instead of prompting for input; combine with
  `--yes` to approve mutations in automation
- `--color=auto|always|never`: control color in human-readable output; `auto`
  honors `NO_COLOR` and only uses color on a terminal
- `--version`: print the current aim version

## Automation contract

`--json` writes command results to stdout and keeps prompts and diagnostics on
stderr. `update` and `selfupdate` still require `--yes` when run
non-interactively and an update is available.

Exit codes are:

- `0`: success, no-op, or user cancellation
- `1`: an operational failure, including a partial bulk-update failure
- `2`: invalid flags or arguments

Update JSON uses explicit statuses including `updated`, `up_to_date`,
`updates_available`, `canceled`, `skipped`, `partial_failure`, and `failed`.
It also includes checked, available, updated, failed, and skipped counts or
collections. A committed operation that could not finish cleanup reports
warnings rather than claiming that the entire operation failed.

## Configuration and data

Run `aim paths` to print the effective paths for the current environment. By
default, aim uses:

- `~/.config/aim/config.toml` for configuration
- `~/.local/share/aim/apps.json` for managed-app state
- `~/.local/share/aim/appimages` for installed AppImages
- `~/.local/share/applications` for desktop entries
- `~/.local/share/icons` for icons

The locations follow `XDG_CONFIG_HOME` and `XDG_DATA_HOME` when those variables
are set. The currently supported configuration setting is:

```toml
appimage_dir = "~/Applications"
```

`appimage_dir` changes where managed AppImages are installed. Desktop entries,
icons, and state continue to use their XDG data locations.

## Support and trust boundaries

- aim runs on Linux. Release archives are currently published for `amd64` and
  `arm64`.
- Desktop integration follows the XDG desktop-entry and icon conventions.
  Refresh behavior depends on the desktop tools available on the host.
- GitHub release sources are the only update sources currently applied.
  Embedded `zsync`, `local_file`, and unsupported metadata is retained for
  inspection but is not applied yet.
- Inspecting, adding, or updating an AppImage executes its extraction and
  update-information modes as the current user. Only manage AppImages you
  trust.
- GitHub downloads use HTTPS and release metadata, but publisher signatures are
  not currently verified by aim.

## Recovery

aim stages integration and update work before replacing managed files. If a
command fails, read the complete error before retrying and run `aim info <id>`
to inspect the recorded paths and source. `aim paths` locates the state file
and managed directories.

Before manually repairing files, back up `apps.json` and the affected managed
artifacts. Mutating workflows coordinate through an operation lock, while
repository writes also use an adjacent `apps.json.lock` file. This prevents
overlapping aim processes from racing artifact and state changes. Automated
diagnosis, repair, cleanup, and user-facing rollback are planned but are not
available yet.

## More help

- `aim --help` for the CLI overview
- `aim help <command>` for command-specific manual pages
- `aim <command> --help` for flags and usage on a specific command
- `man aim` for the full manual page

## License

[MIT](/LICENSE)
