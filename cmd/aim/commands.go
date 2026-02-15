package main

import (
	"bufio"
	"context"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
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

func UpdateCmd(ctx context.Context, cmd *cli.Command) error {
	args := cmd.Args().Slice()
	if len(args) > 0 {
		switch args[0] {
		case "check":
			return UpdateCheckCmd(ctx, cmd, args[1:])
		case "set":
			return UpdateSetCmd(ctx, cmd, args[1:])
		}
	}

	if hasUpdateSetFlags(cmd) {
		return fmt.Errorf("update source flags can only be used with `aim update set`")
	}

	targetID := ""
	if len(args) > 0 {
		targetID = args[0]
	}
	if len(args) > 1 {
		return fmt.Errorf("too many arguments")
	}

	return runManagedUpdate(ctx, cmd, targetID)
}

func UpdateCheckCmd(ctx context.Context, cmd *cli.Command, args []string) error {
	if cmd.IsSet("yes") || cmd.IsSet("check-only") {
		return fmt.Errorf("flags --yes/-y and --check-only/-c are not supported with `aim update check`")
	}
	if len(args) != 1 {
		return fmt.Errorf("usage: aim update check <path-to.AppImage>")
	}

	path := strings.TrimSpace(args[0])
	if !util.HasExtension(path, ".AppImage") {
		return fmt.Errorf("update check only supports local AppImage files; use `aim update <id>` for installed apps")
	}

	absPath, err := util.MakeAbsolute(path)
	if err != nil {
		return err
	}

	info, err := core.GetUpdateInfo(absPath)
	if err != nil {
		return err
	}

	if info.Kind != models.UpdateZsync {
		return fmt.Errorf("unsupported local update info kind %q", info.Kind)
	}

	localSHA1, err := util.Sha1(absPath)
	if err != nil {
		return err
	}

	updater := &models.UpdateSource{
		Kind: models.UpdateZsync,
		Zsync: &models.ZsyncUpdateSource{
			UpdateInfo: info.UpdateInfo,
			Transport:  info.Transport,
		},
	}

	update, err := core.ZsyncUpdateCheck(updater, localSHA1)
	if err != nil {
		return err
	}

	color := useColor(cmd)
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

	return nil
}

func UpdateSetCmd(ctx context.Context, cmd *cli.Command, args []string) error {
	_ = ctx
	if cmd.IsSet("yes") || cmd.IsSet("check-only") {
		return fmt.Errorf("flags --yes/-y and --check-only/-c are not supported with `aim update set`")
	}

	id := ""
	if len(args) > 0 {
		id = strings.TrimSpace(args[0])
	}
	color := useColor(cmd)

	if id == "" {
		return fmt.Errorf("missing required argument <id>")
	}

	incomingSource, err := resolveUpdateSourceFromSetFlags(cmd)
	if err != nil {
		return err
	}

	app, err := repo.GetApp(id)
	if err != nil {
		return err
	}

	if app.Update != nil && app.Update.Kind != models.UpdateNone {
		prompt := fmt.Sprintf("Update source already set to %s:\n%s\nWill be replaced with:\n%s\nOverwrite? [y/N]: ",
			app.Update.Kind,
			updateSummary(app.Update),
			updateSummary(incomingSource),
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

	app.Update = incomingSource

	if err := repo.UpdateApp(app); err != nil {
		return err
	}

	msg := fmt.Sprintf("Update source set: %s", updateSummary(incomingSource))
	fmt.Println(colorize(color, "\033[0;32m", msg))
	return nil
}

func hasUpdateSetFlags(cmd *cli.Command) bool {
	keys := []string{"github", "gitlab", "asset", "zsync-url", "manifest-url", "url", "sha256"}
	for _, key := range keys {
		if cmd.IsSet(key) {
			return true
		}
	}
	return false
}

func resolveUpdateSourceFromSetFlags(cmd *cli.Command) (*models.UpdateSource, error) {
	githubRepo := strings.TrimSpace(cmd.String("github"))
	gitlabProject := strings.TrimSpace(cmd.String("gitlab"))
	assetPattern := strings.TrimSpace(cmd.String("asset"))
	zsyncURL := strings.TrimSpace(cmd.String("zsync-url"))
	manifestURL := strings.TrimSpace(cmd.String("manifest-url"))
	directURL := strings.TrimSpace(cmd.String("url"))
	sha256 := strings.ToLower(strings.TrimSpace(cmd.String("sha256")))

	selectorCount := 0
	for _, value := range []string{githubRepo, gitlabProject, zsyncURL, manifestURL, directURL} {
		if value != "" {
			selectorCount++
		}
	}

	if selectorCount == 0 {
		return nil, fmt.Errorf("missing update source; set one of --github, --gitlab, --zsync-url, --manifest-url, or --url")
	}
	if selectorCount > 1 {
		return nil, fmt.Errorf("update source flags are mutually exclusive")
	}

	if githubRepo != "" {
		if assetPattern == "" {
			return nil, fmt.Errorf("missing required flag --asset")
		}
		if sha256 != "" {
			return nil, fmt.Errorf("--sha256 is only supported with --url")
		}
		return &models.UpdateSource{
			Kind: models.UpdateGitHubRelease,
			GitHubRelease: &models.GitHubReleaseUpdateSource{
				Repo:  githubRepo,
				Asset: assetPattern,
			},
		}, nil
	}

	if gitlabProject != "" {
		if assetPattern == "" {
			return nil, fmt.Errorf("missing required flag --asset")
		}
		if sha256 != "" {
			return nil, fmt.Errorf("--sha256 is only supported with --url")
		}
		return &models.UpdateSource{
			Kind: models.UpdateGitLabRelease,
			GitLabRelease: &models.GitLabReleaseUpdateSource{
				Project: gitlabProject,
				Asset:   assetPattern,
			},
		}, nil
	}

	if zsyncURL != "" {
		if assetPattern != "" {
			return nil, fmt.Errorf("--asset is only supported with --github or --gitlab")
		}
		if sha256 != "" {
			return nil, fmt.Errorf("--sha256 is only supported with --url")
		}
		if !isHTTPSURL(zsyncURL) {
			return nil, fmt.Errorf("--zsync-url must be a valid https URL")
		}
		return &models.UpdateSource{
			Kind: models.UpdateZsync,
			Zsync: &models.ZsyncUpdateSource{
				UpdateInfo: "zsync|" + zsyncURL,
				Transport:  "zsync",
			},
		}, nil
	}

	if manifestURL != "" {
		if assetPattern != "" {
			return nil, fmt.Errorf("--asset is only supported with --github or --gitlab")
		}
		if sha256 != "" {
			return nil, fmt.Errorf("--sha256 is only supported with --url")
		}
		if !isHTTPSURL(manifestURL) {
			return nil, fmt.Errorf("--manifest-url must be a valid https URL")
		}
		return &models.UpdateSource{
			Kind: models.UpdateManifest,
			Manifest: &models.ManifestUpdateSource{
				URL: manifestURL,
			},
		}, nil
	}

	if assetPattern != "" {
		return nil, fmt.Errorf("--asset is only supported with --github or --gitlab")
	}
	if !isHTTPSURL(directURL) {
		return nil, fmt.Errorf("--url must be a valid https URL")
	}
	if sha256 == "" {
		return nil, fmt.Errorf("missing required flag --sha256 for --url")
	}
	if !isValidSHA256(sha256) {
		return nil, fmt.Errorf("--sha256 must be a 64-character hex value")
	}

	return &models.UpdateSource{
		Kind: models.UpdateDirectURL,
		DirectURL: &models.DirectURLUpdateSource{
			URL:    directURL,
			SHA256: sha256,
		},
	}, nil
}

func PinCmd(ctx context.Context, cmd *cli.Command) error {
	_ = ctx
	id := strings.TrimSpace(cmd.StringArg("id"))
	if id == "" {
		return fmt.Errorf("missing required argument <id>")
	}

	app, changed, err := setPinnedState(id, true)
	if err != nil {
		return err
	}

	color := useColor(cmd)
	if changed {
		fmt.Println(colorize(color, "\033[0;32m", fmt.Sprintf("Pinned %s", app.ID)))
		return nil
	}

	fmt.Println(colorize(color, "\033[0;33m", fmt.Sprintf("%s is already pinned", app.ID)))
	return nil
}

func UnpinCmd(ctx context.Context, cmd *cli.Command) error {
	_ = ctx
	id := strings.TrimSpace(cmd.StringArg("id"))
	if id == "" {
		return fmt.Errorf("missing required argument <id>")
	}

	app, changed, err := setPinnedState(id, false)
	if err != nil {
		return err
	}

	color := useColor(cmd)
	if changed {
		fmt.Println(colorize(color, "\033[0;32m", fmt.Sprintf("Unpinned %s", app.ID)))
		return nil
	}

	fmt.Println(colorize(color, "\033[0;33m", fmt.Sprintf("%s is already unpinned", app.ID)))
	return nil
}

func setPinnedState(id string, pinned bool) (*models.App, bool, error) {
	app, err := repo.GetApp(id)
	if err != nil {
		return nil, false, err
	}

	if app.Pinned == pinned {
		return app, false, nil
	}

	app.Pinned = pinned
	app.UpdatedAt = util.NowISO()

	if err := repo.UpdateApp(app); err != nil {
		return nil, false, err
	}

	return app, true, nil
}

type pendingManagedUpdate struct {
	App            *models.App
	URL            string
	Asset          string
	Label          string
	Available      bool
	Latest         string
	ExpectedSHA1   string
	ExpectedSHA256 string
	FromKind       models.UpdateKind
}

func runManagedUpdate(ctx context.Context, cmd *cli.Command, targetID string) error {
	apps, err := collectManagedUpdateTargets(targetID)
	if err != nil {
		return err
	}

	color := useColor(cmd)
	autoApply := cmd.Bool("yes")
	checkOnly := cmd.Bool("check-only")

	var pending []pendingManagedUpdate
	checkFailures := 0
	skippedPinned := 0
	for _, app := range apps {
		update, err := checkAppUpdate(app)
		if err != nil {
			if metaErr := updateCheckMetadata(app, false, app.UpdateAvailable, app.LatestVersion); metaErr != nil {
				if targetID != "" {
					return metaErr
				}
				checkFailures++
				msg := fmt.Sprintf("%s: failed to persist check metadata: %v", app.ID, metaErr)
				fmt.Println(colorize(color, "\033[0;31m", msg))
			}
			if targetID != "" {
				return err
			}
			checkFailures++
			msg := fmt.Sprintf("%s: failed to check updates: %v", app.ID, err)
			fmt.Println(colorize(color, "\033[0;31m", msg))
			continue
		}

		if update == nil {
			if targetID != "" {
				fmt.Println(colorize(color, "\033[0;32m", "No update information available!"))
			}
			continue
		}

		if err := updateCheckMetadata(app, true, update.Available, update.Latest); err != nil {
			if targetID != "" {
				return err
			}
			checkFailures++
			msg := fmt.Sprintf("%s: failed to persist check metadata: %v", app.ID, err)
			fmt.Println(colorize(color, "\033[0;31m", msg))
		}

		if update.URL == "" {
			if targetID != "" {
				fmt.Println(colorize(color, "\033[0;32m", "You are up-to-date!"))
			}
			continue
		}

		msg := fmt.Sprintf("%s\nDownload from %s\nThen integrate it with %s", update.Label, update.URL, integrationHint(update.Asset))
		if targetID == "" {
			header := fmt.Sprintf("[%s]", app.ID)
			fmt.Println(colorize(color, "\033[1m", header))
		}
		fmt.Println(colorize(color, "\033[0;33m", msg))

		if targetID == "" && app.Pinned {
			skippedPinned++
			info := fmt.Sprintf("%s is pinned; skipping apply", app.ID)
			fmt.Println(colorize(color, "\033[0;33m", info))
			continue
		}

		pending = append(pending, *update)
	}

	if len(pending) == 0 {
		if skippedPinned > 0 {
			msg := fmt.Sprintf("No updates applied; %d app(s) are pinned.", skippedPinned)
			fmt.Println(colorize(color, "\033[0;33m", msg))
			return nil
		}
		if checkFailures > 0 {
			fmt.Println(colorize(color, "\033[0;33m", "No updates applied; some checks failed."))
			return nil
		}
		fmt.Println(colorize(color, "\033[0;32m", "You are up-to-date!"))
		return nil
	}

	if checkOnly {
		return nil
	}

	if !autoApply {
		prompt := "Apply available updates? [y/N]: "
		if targetID != "" {
			prompt = fmt.Sprintf("Apply update for %s? [y/N]: ", targetID)
		} else {
			prompt = fmt.Sprintf("Apply %d available updates? [y/N]: ", len(pending))
		}

		confirmed, err := confirmOverwrite(prompt)
		if err != nil {
			return err
		}
		if !confirmed {
			fmt.Println(colorize(color, "\033[0;33m", "No updates applied."))
			return nil
		}
	}

	applyFailures := 0
	for _, item := range pending {
		updatedApp, err := applyManagedUpdate(ctx, item)
		if err != nil {
			applyFailures++
			msg := fmt.Sprintf("%s: failed to apply update: %v", item.App.ID, err)
			fmt.Println(colorize(color, "\033[0;31m", msg))
			continue
		}

		msg := fmt.Sprintf("Updated %s to v%s", updatedApp.ID, updatedApp.Version)
		fmt.Println(colorize(color, "\033[0;32m", msg))
	}

	if applyFailures > 0 {
		return fmt.Errorf("%d update(s) failed", applyFailures)
	}

	if skippedPinned > 0 {
		msg := fmt.Sprintf("Skipped %d pinned app(s).", skippedPinned)
		fmt.Println(colorize(color, "\033[0;33m", msg))
	}

	return nil
}

func collectManagedUpdateTargets(targetID string) ([]*models.App, error) {
	if strings.TrimSpace(targetID) != "" {
		app, err := repo.GetApp(targetID)
		if err != nil {
			return nil, err
		}
		return []*models.App{app}, nil
	}

	allApps, err := repo.GetAllApps()
	if err != nil {
		return nil, err
	}

	ids := make([]string, 0, len(allApps))
	for id := range allApps {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	apps := make([]*models.App, 0, len(ids))
	for _, id := range ids {
		apps = append(apps, allApps[id])
	}

	return apps, nil
}

func checkAppUpdate(app *models.App) (*pendingManagedUpdate, error) {
	if app == nil || app.Update == nil || app.Update.Kind == models.UpdateNone {
		return nil, nil
	}

	switch app.Update.Kind {
	case models.UpdateZsync:
		update, err := core.ZsyncUpdateCheck(app.Update, app.SHA1)
		if err != nil {
			return nil, err
		}
		if update == nil {
			return &pendingManagedUpdate{
				App:       app,
				Available: false,
				Latest:    "",
				FromKind:  models.UpdateZsync,
			}, nil
		}
		if !update.Available {
			return &pendingManagedUpdate{
				App:       app,
				Available: false,
				Latest:    "",
				FromKind:  models.UpdateZsync,
			}, nil
		}
		label := "Newer version found!"
		if update.PreRelease {
			label = "Newer pre-release version found!"
		}
		return &pendingManagedUpdate{
			App:          app,
			URL:          update.DownloadUrl,
			Asset:        update.AssetName,
			Label:        label,
			Available:    true,
			Latest:       "",
			ExpectedSHA1: strings.TrimSpace(update.RemoteSHA1),
			FromKind:     models.UpdateZsync,
		}, nil
	case models.UpdateGitHubRelease:
		update, err := core.GitHubReleaseUpdateCheck(app.Update, app.Version)
		if err != nil {
			return nil, err
		}
		if update == nil {
			return &pendingManagedUpdate{
				App:       app,
				Available: false,
				Latest:    "",
				FromKind:  models.UpdateGitHubRelease,
			}, nil
		}

		latest := strings.TrimSpace(update.TagName)

		if !update.Available {
			return &pendingManagedUpdate{
				App:       app,
				Available: false,
				Latest:    latest,
				FromKind:  models.UpdateGitHubRelease,
			}, nil
		}
		label := "Newer version found!"
		if update.PreRelease {
			label = "Newer pre-release version found!"
		}
		return &pendingManagedUpdate{
			App:       app,
			URL:       update.DownloadUrl,
			Asset:     update.AssetName,
			Label:     label,
			Available: true,
			Latest:    latest,
			FromKind:  models.UpdateGitHubRelease,
		}, nil
	case models.UpdateGitLabRelease:
		update, err := core.GitLabReleaseUpdateCheck(app.Update, app.Version)
		if err != nil {
			return nil, err
		}
		if update == nil {
			return &pendingManagedUpdate{
				App:       app,
				Available: false,
				Latest:    "",
				FromKind:  models.UpdateGitLabRelease,
			}, nil
		}

		latest := strings.TrimSpace(update.TagName)
		if !update.Available {
			return &pendingManagedUpdate{
				App:       app,
				Available: false,
				Latest:    latest,
				FromKind:  models.UpdateGitLabRelease,
			}, nil
		}

		return &pendingManagedUpdate{
			App:       app,
			URL:       update.DownloadURL,
			Asset:     update.AssetName,
			Label:     "Newer version found!",
			Available: true,
			Latest:    latest,
			FromKind:  models.UpdateGitLabRelease,
		}, nil
	case models.UpdateManifest:
		update, err := core.ManifestUpdateCheck(app.Update, app.Version, app.SHA256)
		if err != nil {
			return nil, err
		}
		if update == nil {
			return &pendingManagedUpdate{
				App:       app,
				Available: false,
				Latest:    "",
				FromKind:  models.UpdateManifest,
			}, nil
		}

		latest := strings.TrimSpace(update.Version)
		if !update.Available {
			return &pendingManagedUpdate{
				App:       app,
				Available: false,
				Latest:    latest,
				FromKind:  models.UpdateManifest,
			}, nil
		}

		return &pendingManagedUpdate{
			App:            app,
			URL:            update.DownloadURL,
			Asset:          update.AssetName,
			Label:          "Newer version found!",
			Available:      true,
			Latest:         latest,
			ExpectedSHA256: strings.ToLower(strings.TrimSpace(update.SHA256)),
			FromKind:       models.UpdateManifest,
		}, nil
	case models.UpdateDirectURL:
		update, err := core.DirectURLUpdateCheck(app.Update, app.SHA256)
		if err != nil {
			return nil, err
		}
		if update == nil {
			return &pendingManagedUpdate{
				App:       app,
				Available: false,
				Latest:    "",
				FromKind:  models.UpdateDirectURL,
			}, nil
		}

		if !update.Available {
			return &pendingManagedUpdate{
				App:       app,
				Available: false,
				Latest:    "",
				FromKind:  models.UpdateDirectURL,
			}, nil
		}

		return &pendingManagedUpdate{
			App:            app,
			URL:            update.DownloadURL,
			Asset:          update.AssetName,
			Label:          "Newer version found!",
			Available:      true,
			Latest:         "",
			ExpectedSHA256: strings.ToLower(strings.TrimSpace(update.SHA256)),
			FromKind:       models.UpdateDirectURL,
		}, nil
	default:
		return nil, fmt.Errorf("unsupported update source kind %q", app.Update.Kind)
	}
}

func updateCheckMetadata(app *models.App, checked, available bool, latest string) error {
	if app == nil {
		return nil
	}

	if checked {
		app.UpdateAvailable = available
		app.LatestVersion = strings.TrimSpace(latest)
	}
	app.LastCheckedAt = util.NowISO()

	if err := repo.UpdateApp(app); err != nil {
		return err
	}

	return nil
}

func applyManagedUpdate(ctx context.Context, update pendingManagedUpdate) (*models.App, error) {
	if strings.TrimSpace(update.URL) == "" {
		return nil, fmt.Errorf("missing download URL")
	}

	tempDir, err := os.MkdirTemp("", "aim-update-*")
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = os.RemoveAll(tempDir)
	}()

	fileName := updateDownloadFilename(update.Asset, update.URL)
	downloadPath := filepath.Join(tempDir, fileName)
	if err := downloadUpdateAsset(ctx, update.URL, downloadPath); err != nil {
		return nil, err
	}

	if err := verifyDownloadedUpdate(downloadPath, update); err != nil {
		return nil, err
	}

	app, err := core.IntegrateFromLocalFile(ctx, downloadPath, func(existing, incoming *models.UpdateSource) (bool, error) {
		return false, nil
	})
	if err != nil {
		return nil, err
	}

	return app, nil
}

func verifyDownloadedUpdate(downloadPath string, update pendingManagedUpdate) error {
	expectedSHA256 := strings.ToLower(strings.TrimSpace(update.ExpectedSHA256))
	if expectedSHA256 != "" {
		sum, err := util.Sha256File(downloadPath)
		if err != nil {
			return err
		}
		if strings.ToLower(sum) != expectedSHA256 {
			return fmt.Errorf("downloaded file sha256 mismatch")
		}
	}

	expectedSHA1 := strings.ToLower(strings.TrimSpace(update.ExpectedSHA1))
	if expectedSHA1 != "" {
		sum, err := util.Sha1(downloadPath)
		if err != nil {
			return err
		}
		if strings.ToLower(sum) != expectedSHA1 {
			return fmt.Errorf("downloaded file sha1 mismatch")
		}
	}

	return nil
}

func updateDownloadFilename(assetName, downloadURL string) string {
	name := strings.TrimSpace(filepath.Base(assetName))
	if name == "" || name == "." || name == string(filepath.Separator) {
		name = strings.TrimSpace(filepath.Base(downloadURL))
	}
	if name == "" || name == "." || name == string(filepath.Separator) {
		name = "update.AppImage"
	}
	if !util.HasExtension(name, ".AppImage") {
		name = name + ".AppImage"
	}
	return name
}

func downloadUpdateAsset(ctx context.Context, assetURL, destination string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, assetURL, nil)
	if err != nil {
		return err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return fmt.Errorf("download failed with status %s", resp.Status)
	}

	f, err := os.OpenFile(destination, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()

	if _, err := io.Copy(f, resp.Body); err != nil {
		return err
	}

	return nil
}

func isHTTPSURL(value string) bool {
	u, err := url.Parse(strings.TrimSpace(value))
	if err != nil {
		return false
	}
	if !strings.EqualFold(u.Scheme, "https") {
		return false
	}
	if strings.TrimSpace(u.Host) == "" {
		return false
	}
	return true
}

func isValidSHA256(value string) bool {
	v := strings.ToLower(strings.TrimSpace(value))
	if len(v) != 64 {
		return false
	}
	_, err := hex.DecodeString(v)
	return err == nil
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
		return fmt.Sprintf("| github: %s, asset: %s", update.GitHubRelease.Repo, update.GitHubRelease.Asset)
	case models.UpdateGitLabRelease:
		if update.GitLabRelease == nil {
			return "| gitlab: <missing>"
		}
		return fmt.Sprintf("| gitlab: %s, asset: %s", update.GitLabRelease.Project, update.GitLabRelease.Asset)
	case models.UpdateManifest:
		if update.Manifest == nil {
			return "| manifest: <missing>"
		}
		return fmt.Sprintf("| manifest: %s", update.Manifest.URL)
	case models.UpdateDirectURL:
		if update.DirectURL == nil {
			return "| direct-url: <missing>"
		}
		return fmt.Sprintf("| direct-url: %s", update.DirectURL.URL)
	default:
		return fmt.Sprintf("| %s", update.Kind)
	}
}
