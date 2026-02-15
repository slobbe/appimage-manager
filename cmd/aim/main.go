package main

import (
	"context"
	"log"
	"os"

	"github.com/urfave/cli/v3"

	"github.com/slobbe/appimage-manager/internal/config"
	repo "github.com/slobbe/appimage-manager/internal/repository"
)

var (
	version = "dev" // overridden by ldflags: `go build -ldflags "-X main.version=VERSION" -o ./bin/aim ./cmd/aim`
)

func main() {
	cmd := &cli.Command{
		Name:    "aim",
		Version: version,
		Usage:   "Integrate AppImages into your desktop environment",
		Action:  RootCmd,
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:  "no-color",
				Usage: "disable ANSI color output",
			},
			&cli.BoolFlag{
				Name:  "upgrade",
				Usage: "check and install the latest stable aim release",
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
				Name:      "update",
				Usage:     "Check/apply updates, check local files, or set update source",
				UsageText: "aim update [<id>] [--yes|-y] [--check-only|-c]\n   aim update check <path-to.AppImage>\n   aim update set <id> (--github owner/repo --asset \"*.AppImage\" | --gitlab namespace/project --asset \"*.AppImage\" | --zsync-url <https-url> | --manifest-url <https-url> | --url <https-url> --sha256 <sha256>)",
				Flags: []cli.Flag{
					&cli.BoolFlag{
						Name:    "yes",
						Aliases: []string{"y"},
						Usage:   "apply updates without prompting",
					},
					&cli.BoolFlag{
						Name:    "check-only",
						Aliases: []string{"c"},
						Usage:   "check only; do not apply updates",
					},
					&cli.StringFlag{
						Name:  "github",
						Usage: "GitHub repo in the form owner/repo (for 'update set')",
					},
					&cli.StringFlag{
						Name:  "asset",
						Usage: "asset filename pattern, e.g. \"*.AppImage\" (for 'update set')",
					},
					&cli.StringFlag{
						Name:  "gitlab",
						Usage: "GitLab project path namespace/project (for 'update set')",
					},
					&cli.StringFlag{
						Name:  "zsync-url",
						Usage: "direct zsync metadata URL (https, for 'update set')",
					},
					&cli.StringFlag{
						Name:  "manifest-url",
						Usage: "manifest endpoint URL (https, for 'update set')",
					},
					&cli.StringFlag{
						Name:  "url",
						Usage: "direct AppImage URL (https, for 'update set')",
					},
					&cli.StringFlag{
						Name:  "sha256",
						Usage: "expected SHA-256 for direct URL updates (for 'update set --url')",
					},
				},
				Action: UpdateCmd,
			},
			{
				Name:  "pin",
				Usage: "Pin an app to prevent batch update apply",
				Arguments: []cli.Argument{
					&cli.StringArg{
						Name:      "id",
						UsageText: "<id>",
					},
				},
				Action: PinCmd,
			},
			{
				Name:  "unpin",
				Usage: "Unpin an app so batch update apply can include it",
				Arguments: []cli.Argument{
					&cli.StringArg{
						Name:      "id",
						UsageText: "<id>",
					},
				},
				Action: UnpinCmd,
			},
		},
	}

	if err := config.EnsureDirsExist(); err != nil {
		log.Fatal(err)
	}

	if err := repo.MigrateLegacyToXDG(); err != nil {
		log.Fatal(err)
	}

	if err := cmd.Run(context.Background(), os.Args); err != nil {
		log.Fatal(err)
	}
}
