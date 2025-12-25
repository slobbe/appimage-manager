# AppImage Manager (aim)

A small CLI that integrates AppImages into your desktop environment.

## Build from source

```sh
git clone https://github.com/slobbe/appimage-manager.git
cd appimage-manager
go build -o bin/aim ./cmd/aim
./install.sh
```

## Usage

Integrate AppImage into your desktop environment:

```sh
aim add [-mv] <appimage|id>
```

| Option | Meaning                                  |
| ------ | ---------------------------------------- |
| `-mv`  | Move the AppImage instead of copying it. |

If given an ID of an unlinked AppImage it reintegrates it.

Remove AppImage:

```sh
aim rm [-k] <id>
```

| Option | Meaning                                                   |
| ------ | --------------------------------------------------------- |
| `-k`   | Keep the AppImage files; remove only desktop integration. |

List all integrated AppImages:

```sh
aim list [-a|-i|-u]
```

| Option | Meaning                       |
| ------ | ----------------------------- |
| `-a`   | List all AppImages (default)  |
| `-i`   | List only intgrated AppImages |
| `-u`   | List only unlinked AppImages  |

## License

[MIT](/LICENSE)
