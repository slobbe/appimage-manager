package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/urfave/cli/v3"

	"github.com/slobbe/appimage-manager/internal/config"
	"github.com/slobbe/appimage-manager/internal/core"
	util "github.com/slobbe/appimage-manager/internal/helpers"
	repo "github.com/slobbe/appimage-manager/internal/repository"
	models "github.com/slobbe/appimage-manager/internal/types"
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
				Name:  "t",
				Usage: "test",
				Arguments: []cli.Argument{
					&cli.StringArg{
						Name:      "app",
						UsageText: "<.appimage|id>",
					},
				},
				Action: func(ctx context.Context, cmd *cli.Command) error {
					app, err := core.IntegrateFromLocalFile(ctx, cmd.StringArg("app"))
					if err != nil {
						return err
					}
					
					fmt.Printf(app.Name)
					
					return nil
				},
			},
			{
				Name:  "add",
				Usage: "Integrates AppImage",
				Arguments: []cli.Argument{
					&cli.StringArg{
						Name:      "app",
						UsageText: "<.appimage|id>",
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
				Usage: "Checks for new update",
				Arguments: []cli.Argument{
					&cli.StringArg{
						Name:      "app",
						UsageText: "<.appimage|id>",
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
	input := cmd.StringArg("app")
	
	inputType := identifyInputType(input)
	
	var app *models.App
	
	switch inputType {
	case InputTypeIntegrated:
		appData, err := repo.GetApp(input)
		if err != nil {
			return err
		}
		app = appData
		fmt.Printf("\033[0;32m%s v%s (ID: %s) already integrated!\033[0m\n", app.Name, app.Version, app.ID)
	case InputTypeUnlinked:
		appData, err := core.IntegrateExisting(ctx, input)
		if err != nil {
			return err
		}
		app = appData
		fmt.Printf("\033[0;32mSuccessfully reintegrated %s v%s (ID: %s)\033[0m\n", app.Name, app.Version, app.ID)
	case InputTypeAppImage:
		appData, err := core.IntegrateFromLocalFile(ctx, input)
		if err != nil {
			return err
		}
		app = appData
		fmt.Printf("\033[0;32mSuccessfully integrated %s v%s (ID: %s)\033[0m\n", app.Name, app.Version, app.ID)
	default:
		return fmt.Errorf("unkown argument %s\n", input)
	}
		
	/* 
	if app.Type == "type-2" {
		updateAvailable, downloadLink, _ := core.CheckForUpdate(app.Slug)
		if updateAvailable {
			fmt.Printf("\n\033[0;33mNewer version found!\033[0m\nDownload from \033[1m%s\033[0m\nThen integrate it with `aim add path/to/new.AppImage`\n", downloadLink)
		}
	}
	*/

	return nil
}

func RemoveCmd(ctx context.Context, cmd *cli.Command) error {
	id := cmd.StringArg("id")
	keep := cmd.Bool("keep")

	app, err := core.Remove(ctx, id, keep)

	if err == nil {
		fmt.Printf("\033[0;32mSuccessfully removed %s\033[0m\n", app.Name)
	}

	return err
}

func ListCmd(ctx context.Context, cmd *cli.Command) error {
	all := cmd.Bool("all")
	integrated := cmd.Bool("integrated")
	unlinked := cmd.Bool("unlinked")

	if all == integrated && integrated == unlinked {
		all = true
	}

	
	apps, err := repo.GetAllApps()
	if err != nil {
		return err
	}

	fmt.Printf("\033[1m\033[4m%-15s %-20s %-15s\033[0m\n", "ID", "App Name", "Version")

	if all || integrated {
		for _, app := range apps {
			if len(app.DesktopEntryLink) > 0 {
				fmt.Fprintf(os.Stdout, "%-15s %-20s %-15s\n", app.ID, app.Name, app.Version)
			}
		}
	}

	if all || unlinked {
		for _, app := range apps {
			if len(app.DesktopEntryLink) == 0 {
				fmt.Fprintf(os.Stdout, "\033[2m\033[3m%-15s %-20s %-15s\033[0m\n", app.ID, app.Name, app.Version)
			}
		}
	}

	return nil
}

func CheckCmd(ctx context.Context, cmd *cli.Command) error {
	/* 
	updateAvailable, downloadLink, err := core.CheckForUpdate(cmd.StringArg("app"))

	if err != nil {
		fmt.Printf("\033[0;31mUnable to retrieve update information\033[0m\n")
		return err
	}

	if updateAvailable {
		fmt.Printf("\033[0;33mUpdate available!\033[0m\nDownload from \033[1m%s\033[0m\nThen integrate it with `aim add path/to/new.AppImage`\n", downloadLink)
	} else {
		fmt.Printf("\033[0;32mYou are up-to-date!\033[0m\n")
	}
*/
	return nil
}

const (
	InputTypeAppImage   string = "appimage"
	InputTypeUnlinked   string = "unlinked"
	InputTypeIntegrated string = "integrated"
	InputTypeUnknown    string = "unknown"
)

func identifyInputType(input string) string {
	if util.HasExtension(input, ".AppImage") {
		return InputTypeAppImage
	}
	
	app, err := repo.GetApp(input)
	if err != nil {
		return InputTypeUnknown
	}
	
	if app.DesktopEntryLink == "" {
		return InputTypeUnlinked
	} else {
		return InputTypeIntegrated
	}
}
