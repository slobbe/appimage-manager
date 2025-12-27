package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/urfave/cli/v3"

	"github.com/slobbe/appimage-manager/internal/config"
	"github.com/slobbe/appimage-manager/internal/core"
)

var (
	version = "dev" // overridden by ldflags: `go build -ldflags "-X main.version=VERSION" -o ./bin/aim ./cmd/aim`
)

func main() {
	cmd := &cli.Command{
		Name:    "aim",
		Version: version,
		Usage:   "Easily integrate AppImages into your desktop environment",
		Commands: []*cli.Command{
			{
				Name:  "add",
				Usage: "Integrates AppImage",
				Arguments: []cli.Argument{
					&cli.StringArg{
						Name:      "app",
						UsageText: "<.appimage|id>",
					},
				},
				Flags: []cli.Flag{
					&cli.BoolFlag{
						Name:    "move",
						Aliases: []string{"m"},
						Value:   false,
						Usage:   "move the AppImage instead of copying it",
					},
				},
				Action: AddCmd,
			},
			{
				Name:    "remove",
				Aliases: []string{"rm"},
				Usage:   "Removes AppImage",
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
						Usage:   "keep AppImage files; remove only desktop integration",
					},
				},
				Action: RemoveCmd,
			},
			{
				Name:    "list",
				Aliases: []string{"ls"},
				Usage:   "Lists all AppImages",
				Flags: []cli.Flag{
					&cli.BoolFlag{
						Name:    "all",
						Aliases: []string{"a"},
						Value:   false,
						Usage:   "list all integrated and unlinked AppImages (default)",
					},
					&cli.BoolFlag{
						Name:    "integrated",
						Aliases: []string{"i"},
						Value:   false,
						Usage:   "list only integrated AppImages",
					},
					&cli.BoolFlag{
						Name:    "unlinked",
						Aliases: []string{"u"},
						Value:   false,
						Usage:   "list only unlinked AppImages",
					},
				},
				Action: ListCmd,
			},
			{
				Name:  "check",
				Usage: "Check AppImage for new update",
				Arguments: []cli.Argument{
					&cli.StringArg{
						Name:      "app",
						UsageText: "<.appimage>",
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

func AddCmd(ctx context.Context, cmd *cli.Command) error {
	appImage := cmd.StringArg("app")
	move := cmd.Bool("move")

	return core.IntegrateAppImage(appImage, move)
}

func RemoveCmd(ctx context.Context, cmd *cli.Command) error {
	id := cmd.StringArg("id")
	keep := cmd.Bool("keep")

	return core.RemoveAppImage(id, keep)
}

func ListCmd(ctx context.Context, cmd *cli.Command) error {
	all := cmd.Bool("all")
	integrated := cmd.Bool("integrated")
	unlinked := cmd.Bool("unlinked")

	if all == integrated && integrated == unlinked {
		all = true
	}

	db, err := core.LoadDB(config.DbSrc)
	if err != nil {
		return err
	}

	apps := db.Apps

	fmt.Printf("\033[1m\033[4m%-15s %-20s %-15s %-10s\033[0m\n", "ID", "App Name", "Version", "Added")

	addedAtFormat := time.DateOnly

	if all || integrated {
		for _, app := range apps {
			if len(app.DesktopLink) > 0 {
				addedAt, _ := time.Parse(time.RFC3339, app.AddedAt)
				fmt.Fprintf(os.Stdout, "%-15s %-20s %-15s %-10s\n", app.Slug, app.Name, app.Version, addedAt.Format(addedAtFormat))
			}
		}
	}

	if all || unlinked {
		for _, app := range apps {
			if len(app.DesktopLink) == 0 {
				addedAt, _ := time.Parse(time.RFC3339, app.AddedAt)
				fmt.Fprintf(os.Stdout, "\033[2m\033[3m%-15s %-20s %-15s %-10s\033[0m\n", app.Slug, app.Name, app.Version, addedAt.Format(addedAtFormat))
			}
		}
	}

	return nil
}

func CheckCmd(ctx context.Context, cmd *cli.Command) error {
	app := cmd.StringArg("app")
	
	db, err := core.LoadDB(config.DbSrc)
	if err != nil {
		return err
	}
	
	_, src, err := core.IdentifyInput(app, db)
	if err != nil {
		return err
	}
	
	info, err := core.GetUpdateInfo(src)
	if err != nil {
		return err
	}

	url, err := core.ResolveUpdateInfo(info)
	if err != nil {
		return err
	}
		
	updateAvailable, err := core.IsUpdateAvailable(src, url)
	if err != nil {
		return err
	}
	
	if updateAvailable {
		fmt.Printf("Update Available!\n")
	} else {
		fmt.Printf("No updates found!\n")
	}
	
	return nil
}
