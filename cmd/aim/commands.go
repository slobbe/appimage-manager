package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/slobbe/appimage-manager/internal/core"
	util "github.com/slobbe/appimage-manager/internal/helpers"
	repo "github.com/slobbe/appimage-manager/internal/repository"
	models "github.com/slobbe/appimage-manager/internal/types"
	"github.com/urfave/cli/v3"
)

func RootCmd(ctx context.Context, cmd *cli.Command) error {
	if !cmd.Bool("upgrade") {
		return cli.ShowRootCommandHelp(cmd)
	}

	color := useColor(cmd)
	result, err := core.SelfUpgrade(ctx, version)
	if err != nil {
		return err
	}

	if result.Updated {
		msg := fmt.Sprintf("Updated aim from v%s to v%s", result.CurrentVersion, result.LatestVersion)
		fmt.Println(colorize(color, "\033[0;32m", msg))
		return nil
	}

	msg := fmt.Sprintf("aim is already up to date (v%s)", result.CurrentVersion)
	fmt.Println(colorize(color, "\033[0;32m", msg))
	return nil
}

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
		appData, err := core.IntegrateFromLocalFile(ctx, input, func(existing, incoming *models.UpdateSource) (bool, error) {
			prompt := fmt.Sprintf("Update source already set to %s:\n%s\nWill be replaced with:\n%s\nOverwrite with AppImage info? [y/N]: ", existing.Kind, updateSummary(existing), updateSummary(incoming))
			return confirmOverwrite(prompt)
		})
		if err != nil {
			return err
		}
		app = appData
		msg := fmt.Sprintf("Successfully integrated %s v%s (ID: %s)", app.Name, app.Version, app.ID)
		fmt.Println(colorize(color, "\033[0;32m", msg))
	default:
		return fmt.Errorf("unknown argument %s", input)
	}

	if app.Update.Kind == models.UpdateZsync {
		update, err := core.ZsyncUpdateCheck(app.Update, app.SHA1)

		if update != nil && update.Available {
			msg := fmt.Sprintf("Newer version found!\nDownload from %s\nThen integrate it with %s", update.DownloadUrl, integrationHint(update.AssetName))
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

		if app.Update == nil || app.Update.Kind == models.UpdateNone {
			fmt.Printf("No update information available!\n")
			return nil
		}

		if app.Update.Kind == models.UpdateZsync {
			update, err := core.ZsyncUpdateCheck(app.Update, app.SHA1)

			if update != nil && update.Available {
				label := "Newer version found!"
				if update.PreRelease {
					label = "Newer pre-release version found!"
				}
				msg := fmt.Sprintf("%s\nDownload from %s\nThen integrate it with %s", label, update.DownloadUrl, integrationHint(update.AssetName))
				fmt.Println(colorize(color, "\033[0;33m", msg))
			} else {
				fmt.Println(colorize(color, "\033[0;32m", "You are up-to-date!"))
			}

			return err
		}

		if app.Update.Kind == models.UpdateGitHubRelease {
			update, err := core.GitHubReleaseUpdateCheck(app.Update, app.Version)
			if err != nil {
				return err
			}

			if update != nil && update.Available {
				label := "Newer version found!"
				if update.PreRelease {
					label = "Newer pre-release version found!"
				}
				releaseLabel := "latest"
				if update.PreRelease {
					releaseLabel = "pre"
				}
				msg := fmt.Sprintf("%s (release: %s)\nDownload from %s\nThen integrate it with %s", label, releaseLabel, update.DownloadUrl, integrationHint(update.AssetName))
				fmt.Println(colorize(color, "\033[0;33m", msg))
			} else {
				releaseLabel := "latest"
				if update != nil && update.PreRelease {
					releaseLabel = "pre"
				}
				msg := fmt.Sprintf("You are up-to-date! (release: %s)", releaseLabel)
				fmt.Println(colorize(color, "\033[0;32m", msg))
			}

			return nil
		}

		fmt.Printf("No update information available!\n")
	case InputTypeAppImage:
		// TODO: support update checks for local AppImage files.
		return fmt.Errorf("checking updates for local AppImages is not supported yet")
	default:
		return fmt.Errorf("unknown argument %s", input)
	}

	return nil
}

func UpdateSetCmd(ctx context.Context, cmd *cli.Command) error {
	id := cmd.StringArg("id")
	repoSlug := cmd.String("github")
	assetPattern := cmd.String("asset")
	preRelease := cmd.Bool("pre-release")
	color := useColor(cmd)

	if id == "" {
		return fmt.Errorf("missing required argument <id>")
	}

	if repoSlug == "" {
		return fmt.Errorf("missing required flag --github")
	}

	if assetPattern == "" {
		return fmt.Errorf("missing required flag --asset")
	}

	app, err := repo.GetApp(id)
	if err != nil {
		return err
	}

	if app.Update != nil && app.Update.Kind != models.UpdateNone {
		prompt := fmt.Sprintf("Update source already set to %s:\n%s\nWill be replaced with:\n%s\nOverwrite? [y/N]: ",
			app.Update.Kind,
			updateSummary(app.Update),
			updateSummary(&models.UpdateSource{
				Kind: models.UpdateGitHubRelease,
				GitHubRelease: &models.GitHubReleaseUpdateSource{
					Repo:        repoSlug,
					Asset:       assetPattern,
					ReleaseKind: releaseKind(preRelease),
				},
			}),
		)
		confirmed, err := confirmOverwrite(prompt)
		if err != nil {
			return err
		}
		if !confirmed {
			fmt.Println(colorize(color, "\033[0;33m", "Update source unchanged."))
			return nil
		}
	}

	app.Update = &models.UpdateSource{
		Kind: models.UpdateGitHubRelease,
		GitHubRelease: &models.GitHubReleaseUpdateSource{
			Repo:        repoSlug,
			Asset:       assetPattern,
			ReleaseKind: releaseKind(preRelease),
		},
	}

	if err := repo.UpdateApp(app); err != nil {
		return err
	}

	msg := fmt.Sprintf("Update source set to GitHub releases: %s (pattern: %s, release: %s)", repoSlug, assetPattern, releaseKind(preRelease))
	fmt.Println(colorize(color, "\033[0;32m", msg))
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

func confirmOverwrite(prompt string) (bool, error) {
	fmt.Print(prompt)
	reader := bufio.NewReader(os.Stdin)
	line, err := reader.ReadString('\n')
	if err != nil {
		return false, err
	}

	answer := strings.TrimSpace(line)
	return strings.EqualFold(answer, "y") || strings.EqualFold(answer, "yes"), nil
}

func integrationHint(assetName string) string {
	if strings.TrimSpace(assetName) == "" {
		return "`aim add path/to/new.AppImage`"
	}
	return fmt.Sprintf("`aim add path/to/%s`", assetName)
}

func updateSummary(update *models.UpdateSource) string {
	if update == nil {
		return ""
	}

	switch update.Kind {
	case models.UpdateZsync:
		if update.Zsync == nil {
			return "| zsync: <missing>"
		}
		if update.Zsync.UpdateInfo != "" {
			return fmt.Sprintf("| zsync: %s", update.Zsync.UpdateInfo)
		}
		return "| zsync"
	case models.UpdateGitHubRelease:
		if update.GitHubRelease == nil {
			return "| github: <missing>"
		}
		release := update.GitHubRelease.ReleaseKind
		if release == "" {
			release = "latest"
		}
		return fmt.Sprintf("| github: %s, asset: %s, release: %s", update.GitHubRelease.Repo, update.GitHubRelease.Asset, release)
	default:
		return fmt.Sprintf("| %s", update.Kind)
	}
}

func releaseKind(preRelease bool) string {
	if preRelease {
		return "pre"
	}
	return "latest"
}
