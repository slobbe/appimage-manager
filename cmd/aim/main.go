package main

import (
	"context"
	"log"
	"os"

	"github.com/urfave/cli/v3"

	"github.com/slobbe/appimage-manager/internal/config"
)

var (
	version = "dev" // overridden by ldflags: `go build -ldflags "-X main.version=VERSION" -o ./bin/aim ./cmd/aim`
)

func main() {
	cmd := &cli.Command{
		Name:    "aim",
		Version: version,
		Usage:   "Integrate AppImages into your desktop environment",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:  "no-color",
				Usage: "disable ANSI color output",
			},
		},
		Commands: []*cli.Command{
			{
				Name:  "add",
				Usage: "Integrate an AppImage from a file path or existing ID",
				Arguments: []cli.Argument{
					&cli.StringArg{
						Name:      "app",
						UsageText: "<path-to.AppImage|id>",
					},
				},
				Action: AddCmd,
			},
			{
				Name:    "remove",
				Aliases: []string{"rm"},
				Usage:   "Remove an integrated AppImage by ID",
				Arguments: []cli.Argument{
					&cli.StringArg{
						Name:      "id",
						UsageText: "<id>",
					},
				},
				Flags: []cli.Flag{
					&cli.BoolFlag{
						Name:    "keep",
						Aliases: []string{"k"},
						Value:   false,
						Usage:   "keep AppImage file; remove only desktop integration",
					},
				},
				Action: RemoveCmd,
			},
			{
				Name:    "list",
				Aliases: []string{"ls"},
				Usage:   "List AppImages",
				Flags: []cli.Flag{
					&cli.BoolFlag{
						Name:    "all",
						Aliases: []string{"a"},
						Value:   false,
						Usage:   "list all AppImages (default)",
					},
					&cli.BoolFlag{
						Name:    "integrated",
						Aliases: []string{"i"},
						Value:   false,
						Usage:   "list integrated AppImages only",
					},
					&cli.BoolFlag{
						Name:    "unlinked",
						Aliases: []string{"u"},
						Value:   false,
						Usage:   "list unlinked AppImages only",
					},
				},
				Action: ListCmd,
			},
			{
				Name:  "check",
				Usage: "Check for updates by AppImage ID",
				Arguments: []cli.Argument{
					&cli.StringArg{
						Name:      "app",
						UsageText: "<id>",
					},
				},
				Action: CheckCmd,
			},
		},
	}

	if err := config.EnsureDirsExist(); err != nil {
		log.Fatal(err)
	}

	if err := cmd.Run(context.Background(), os.Args); err != nil {
		log.Fatal(err)
	}
}
