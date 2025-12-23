# AppImage Manager

A simple AppImage manager CLI written in Go.

## Build

```sh
go build -o ./bin/aim ./cmd/aim
```

## Usage

Add AppImage from system:

```sh
bin/aim add [OPTIONS] <app-image>
```

| Option | Meaning |
|--------|---------|
| `-mv` | Move the AppImage instead of copying it. |
| `-a`  | AppImage is given as an absolute path. |

Remove AppImage from system:

```sh
bin/aim rm [-keep] <id>
```

List integrated AppImages:

```sh
bin/aim list
```

## License

[MIT](/LICENSE)
