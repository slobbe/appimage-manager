# AppImage Manager (aim)

A small CLI tool to easily integrate AppImages into your desktop environment.

## Build from source

```sh
git clone https://github.com/slobbe/appimage-manager.git
cd appimage-manager
go build -o ./bin/aim ./cmd/aim
./install.sh
```

## Usage

**Integrate** AppImage into your desktop environment:

```sh
aim add [options] <.appimage|id>
```

If given an ID of an unlinked AppImage it reintegrates it.

| Option         | Meaning                                 |
| :------------- | :-------------------------------------- |
| `--move`, `-m` | move the AppImage instead of copying it |

**Remove** AppImage:

```sh
aim remove [options] <id>
```

| Option         | Meaning                                                  |
| :------------- | :------------------------------------------------------- |
| `--keep`, `-k` | keep the AppImage files; remove only desktop integration |

**List** all integrated AppImages:

```sh
aim list [options]
```

| Option               | Meaning                       |
| :------------------- | :---------------------------- |
| `--all`, `-a`        | list all AppImages (default)  |
| `--integrated`, `-i` | list only intgrated AppImages |
| `--unlinked`, `-u`   | list only unlinked AppImages  |

## License

[MIT](/LICENSE)
