# AppImage Manager

A simple AppImage manager CLI written in Go.

## Build

```sh
go build -o ./bin/aim ./cmd/aim
```

## Usage

Add AppImage from system:

```sh
bin/aim add [--move] <appimage>
```

Remove AppImage from system:

```sh
bin/aim rm [--keep] <id>
```

List integrated AppImages:

```sh
bin/aim list
```
