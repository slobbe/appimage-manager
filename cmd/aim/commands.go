package main

import (
	"context"
	"fmt"
	"os"

	"github.com/slobbe/appimage-manager/internal/core"
	util "github.com/slobbe/appimage-manager/internal/helpers"
	repo "github.com/slobbe/appimage-manager/internal/repository"
	models "github.com/slobbe/appimage-manager/internal/types"
	"github.com/urfave/cli/v3"
)

func AddCmd(ctx context.Context, cmd *cli.Command) error {
	input := cmd.StringArg("app")
	color := useColor(cmd)

	inputType := identifyInputType(input)

	var app *models.App

	switch inputType {
	case InputTypeIntegrated:
		appData, err := repo.GetApp(input)
		if err != nil {
			return err
		}
		app = appData
		msg := fmt.Sprintf("%s v%s (ID: %s) already integrated!", app.Name, app.Version, app.ID)
		fmt.Println(colorize(color, "\033[0;32m", msg))
	case InputTypeUnlinked:
		appData, err := core.IntegrateExisting(ctx, input)
		if err != nil {
			return err
		}
		app = appData
		msg := fmt.Sprintf("Successfully reintegrated %s v%s (ID: %s)", app.Name, app.Version, app.ID)
		fmt.Println(colorize(color, "\033[0;32m", msg))
	case InputTypeAppImage:
		appData, err := core.IntegrateFromLocalFile(ctx, input)
		if err != nil {
			return err
		}
		app = appData
		msg := fmt.Sprintf("Successfully integrated %s v%s (ID: %s)", app.Name, app.Version, app.ID)
		fmt.Println(colorize(color, "\033[0;32m", msg))
	default:
		return fmt.Errorf("unknown argument %s", input)
	}

	if app.Update.Kind == "zsync" {
		update, err := core.ZsyncUpdateCheck(app.Update, app.SHA1)

		if update != nil && update.Available {
			msg := fmt.Sprintf("Newer version found!\nDownload from %s\nThen integrate it with `aim add path/to/new.AppImage`", update.DownloadUrl)
			fmt.Printf("\n%s\n", colorize(color, "\033[0;33m", msg))
		}

		return err
	}

	return nil
}

func RemoveCmd(ctx context.Context, cmd *cli.Command) error {
	id := cmd.StringArg("id")
	keep := cmd.Bool("keep")
	color := useColor(cmd)

	app, err := core.Remove(ctx, id, keep)

	if err == nil {
		msg := fmt.Sprintf("Successfully removed %s", app.Name)
		fmt.Println(colorize(color, "\033[0;32m", msg))
	}

	return err
}

func ListCmd(ctx context.Context, cmd *cli.Command) error {
	all := cmd.Bool("all")
	integrated := cmd.Bool("integrated")
	unlinked := cmd.Bool("unlinked")
	color := useColor(cmd)

	if (integrated && unlinked) || (all && (integrated || unlinked)) {
		return fmt.Errorf("flags --all, --integrated, and --unlinked are mutually exclusive")
	}

	if !all && !integrated && !unlinked {
		all = true
	}

	apps, err := repo.GetAllApps()
	if err != nil {
		return err
	}

	header := fmt.Sprintf("%-15s %-20s %-15s", "ID", "App Name", "Version")
	fmt.Println(colorize(color, "\033[1m\033[4m", header))

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
				row := fmt.Sprintf("%-15s %-20s %-15s", app.ID, app.Name, app.Version)
				fmt.Println(colorize(color, "\033[2m\033[3m", row))
			}
		}
	}

	return nil
}

func CheckCmd(ctx context.Context, cmd *cli.Command) error {
	input := cmd.StringArg("app")
	color := useColor(cmd)

	inputType := identifyInputType(input)

	switch inputType {
	case InputTypeIntegrated, InputTypeUnlinked:
		app, err := repo.GetApp(input)
		if err != nil {
			return err
		}

		if app.Update.Kind == "zsync" {
			update, err := core.ZsyncUpdateCheck(app.Update, app.SHA1)

			if update != nil && update.Available {
				msg := fmt.Sprintf("Newer version found!\nDownload from %s\nThen integrate it with `aim add path/to/new.AppImage`", update.DownloadUrl)
				fmt.Println(colorize(color, "\033[0;33m", msg))
			} else {
				fmt.Println(colorize(color, "\033[0;32m", "You are up-to-date!"))
			}

			return err
		} else {
			fmt.Printf("No update information available!\n")
		}
	case InputTypeAppImage:
		// TODO: support update checks for local AppImage files.
		return fmt.Errorf("checking updates for local AppImages is not supported yet")
	default:
		return fmt.Errorf("unknown argument %s", input)
	}

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

func useColor(cmd *cli.Command) bool {
	if cmd.Bool("no-color") {
		return false
	}

	stat, err := os.Stdout.Stat()
	if err != nil {
		return false
	}

	return (stat.Mode() & os.ModeCharDevice) != 0
}

func colorize(enabled bool, code, value string) string {
	if !enabled {
		return value
	}

	return code + value + "\033[0m"
}
