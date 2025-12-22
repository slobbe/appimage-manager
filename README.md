# AppImage Manager

A simple AppImage manager CLI written in Go.

## Build

```sh
go build -o ./bin/aim ./cmd/aim
```

## Usage

Add AppImage from system:

```sh
bin/aim add [--move] <AppImage>
```

Remove AppImage from system:

```sh
bin/aim rm [--keep] <AppImage>
```

List integrated AppImages:

```sh
bin/aim list <AppImage>
```
