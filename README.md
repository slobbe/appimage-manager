# AppImage Manager (aim)

Tiny CLI that integrates AppImages into your desktop environment.

## Build from source

```sh
git clone https://github.com/slobbe/appimage-manager.git
cd appimage-manager
go build -o bin/aim ./cmd/aim
./install.sh          # copies binary to ~/.local/bin
```

## Usage

Integrate AppImage into your desktop environment:

```sh
aim add [OPTIONS] <appimage>
```

| Option | Meaning                                  |
| ------ | ---------------------------------------- |
| `-mv`  | Move the AppImage instead of copying it. |
| `-a`   | AppImage is given as an absolute path.   |

Remove AppImage:

```sh
aim rm [-k] <id>
```

| Option | Meaning                                                  |
| ------ | -------------------------------------------------------- |
| `-k`   | Keep the AppImage file; remove only desktop integration. |

List all integrated AppImages:

```sh
aim list
```

## License

[MIT](/LICENSE)
