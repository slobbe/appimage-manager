package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/slobbe/appimage-manager/internal/core"
	util "github.com/slobbe/appimage-manager/internal/helpers"
	repo "github.com/slobbe/appimage-manager/internal/repository"
	models "github.com/slobbe/appimage-manager/internal/types"
	"github.com/urfave/cli/v3"
)

func RootCmd(ctx context.Context, cmd *cli.Command) error {
	_ = ctx
	return cli.ShowRootCommandHelp(cmd)
}

func UpgradeCmd(ctx context.Context, cmd *cli.Command) error {
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
		msg := fmt.Sprintf("%s %s (ID: %s) already integrated!", app.Name, displayVersion(app.Version), app.ID)
		fmt.Println(colorize(color, "\033[0;32m", msg))
	case InputTypeUnlinked:
		appData, err := core.IntegrateExisting(ctx, input)
		if err != nil {
			return err
		}
		app = appData
		msg := fmt.Sprintf("Successfully reintegrated %s %s (ID: %s)", app.Name, displayVersion(app.Version), app.ID)
		fmt.Println(colorize(color, "\033[0;32m", msg))
	case InputTypeAppImage:
		inputLabel := strings.TrimSpace(filepath.Base(input))
		if inputLabel == "" || inputLabel == "." || inputLabel == string(filepath.Separator) {
			inputLabel = strings.TrimSpace(input)
		}

		status := fmt.Sprintf("Integrating %s...", inputLabel)
		fmt.Println(colorize(color, "\033[0;36m", status))

		appData, err := core.IntegrateFromLocalFile(ctx, input, func(existing, incoming *models.UpdateSource) (bool, error) {
			prompt := fmt.Sprintf("Update source already set to %s:\n%s\nWill be replaced with:\n%s\nOverwrite with AppImage info? [y/N]: ", existing.Kind, updateSummary(existing), updateSummary(incoming))
			return confirmOverwrite(prompt)
		})
		if err != nil {
			return err
		}
		app = appData
		msg := fmt.Sprintf("Successfully integrated %s %s (ID: %s)", app.Name, displayVersion(app.Version), app.ID)
		fmt.Println(colorize(color, "\033[0;32m", msg))
	default:
		return fmt.Errorf("unknown argument %s", input)
	}

	if app.Update.Kind == models.UpdateZsync && cmd.Bool("post-check") {
		update, err := runZsyncUpdateCheck(app.Update, app.SHA1)

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
	unlink := cmd.Bool("unlink")
	color := useColor(cmd)

	app, err := core.Remove(ctx, id, unlink)

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

	update, err := runZsyncUpdateCheck(updater, localSHA1)
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
	sha256 := strings.TrimSpace(cmd.String("sha256"))

	if manifestURL != "" {
		return nil, fmt.Errorf("--manifest-url is no longer supported; use --github, --gitlab, or --zsync-url")
	}
	if directURL != "" {
		return nil, fmt.Errorf("--url is no longer supported; use --github, --gitlab, or --zsync-url")
	}
	if sha256 != "" {
		return nil, fmt.Errorf("--sha256 is no longer supported; use --github, --gitlab, or --zsync-url")
	}

	selectorCount := 0
	for _, value := range []string{githubRepo, gitlabProject, zsyncURL} {
		if value != "" {
			selectorCount++
		}
	}

	if selectorCount == 0 {
		return nil, fmt.Errorf("missing update source; set one of --github, --gitlab, or --zsync-url")
	}
	if selectorCount > 1 {
		return nil, fmt.Errorf("update source flags are mutually exclusive")
	}

	if githubRepo != "" {
		if assetPattern == "" {
			assetPattern = defaultReleaseAssetPattern
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
			assetPattern = defaultReleaseAssetPattern
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

	if assetPattern != "" {
		return nil, fmt.Errorf("--asset is only supported with --github or --gitlab")
	}
	return nil, fmt.Errorf("missing update source; set one of --github, --gitlab, or --zsync-url")
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

type managedCheckResult struct {
	app    *models.App
	update *pendingManagedUpdate
	err    error
}

var runAppUpdateCheck = checkAppUpdate
var runZsyncUpdateCheck = core.ZsyncUpdateCheck
var addAppsBatch = repo.AddAppsBatch
var addSingleApp = repo.AddApp

const defaultReleaseAssetPattern = "*.AppImage"

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
	singleStatusPrinted := false
	checkResults := runManagedChecks(apps)
	metadataUpdates := make([]repo.CheckMetadataUpdate, 0, len(checkResults))

	for _, result := range checkResults {
		app := result.app
		update := result.update
		err := result.err
		if err != nil {
			metadataUpdates = append(metadataUpdates, repo.CheckMetadataUpdate{
				ID:            app.ID,
				Checked:       false,
				Available:     app.UpdateAvailable,
				Latest:        app.LatestVersion,
				LastCheckedAt: util.NowISO(),
			})

			if targetID != "" {
				if metaErr := flushManagedCheckMetadata(metadataUpdates); metaErr != nil {
					return metaErr
				}
				metadataUpdates = metadataUpdates[:0]
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
				singleStatusPrinted = true
			}
			continue
		}

		metadataUpdates = append(metadataUpdates, repo.CheckMetadataUpdate{
			ID:            app.ID,
			Checked:       true,
			Available:     update.Available,
			Latest:        update.Latest,
			LastCheckedAt: util.NowISO(),
		})

		if targetID != "" {
			if metaErr := flushManagedCheckMetadata(metadataUpdates); metaErr != nil {
				return metaErr
			}
			metadataUpdates = metadataUpdates[:0]
		}

		if update.URL == "" {
			if targetID != "" {
				fmt.Println(colorize(color, "\033[0;32m", "You are up-to-date!"))
				singleStatusPrinted = true
			}
			continue
		}

		msg := buildManagedUpdateMessage(*update, checkOnly)
		if targetID == "" {
			header := fmt.Sprintf("[%s]", app.ID)
			fmt.Println(colorize(color, "\033[1m", header))
		}
		fmt.Println(colorize(color, "\033[0;33m", msg))

		pending = append(pending, *update)
	}

	if err := flushManagedCheckMetadata(metadataUpdates); err != nil {
		if targetID != "" {
			return err
		}
		checkFailures++
		msg := fmt.Sprintf("failed to persist check metadata: %v", err)
		fmt.Println(colorize(color, "\033[0;31m", msg))
	}

	if len(pending) == 0 {
		if targetID != "" && singleStatusPrinted {
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
	applySuccesses := 0
	totalPending := len(pending)
	appliedApps := make([]*models.App, 0, totalPending)
	interactiveProgress := isTerminalOutput()
	for i, item := range pending {
		progress := fmt.Sprintf("Updating %s (%d/%d)", item.App.ID, i+1, totalPending)
		if transition := updateVersionTransition(item); transition != "" {
			progress = fmt.Sprintf("%s %s", progress, transition)
		}
		fmt.Println(colorize(color, "\033[0;36m", progress))

		updatedApp, err := applyManagedUpdate(ctx, item, interactiveProgress)
		if err != nil {
			applyFailures++
			msg := fmt.Sprintf("%s: failed to apply update: %v", item.App.ID, err)
			fmt.Println(colorize(color, "\033[0;31m", msg))
			continue
		}

		msg := fmt.Sprintf("Updated %s to %s", updatedApp.ID, displayVersion(updatedApp.Version))
		fmt.Println(colorize(color, "\033[0;32m", msg))
		applySuccesses++
		appliedApps = append(appliedApps, updatedApp)
	}

	if applySuccesses > 0 {
		core.RefreshDesktopIntegrationCaches(ctx)
	}

	persistErr := persistManagedAppliedApps(appliedApps)

	if applyFailures > 0 {
		if persistErr != nil {
			return fmt.Errorf("%d update(s) failed; failed to persist applied updates: %w", applyFailures, persistErr)
		}
		return fmt.Errorf("%d update(s) failed", applyFailures)
	}

	if persistErr != nil {
		return persistErr
	}

	return nil
}

func runManagedChecks(apps []*models.App) []managedCheckResult {
	results := make([]managedCheckResult, len(apps))
	if len(apps) == 0 {
		return results
	}

	groups := make(map[string][]int, len(apps))
	orderedKeys := make([]string, 0, len(apps))
	for idx, app := range apps {
		key := managedCheckCacheKey(app, idx)
		if _, exists := groups[key]; !exists {
			orderedKeys = append(orderedKeys, key)
		}
		groups[key] = append(groups[key], idx)
	}

	jobs := make(chan int, len(orderedKeys))
	workerCount := managedCheckWorkerCount(len(orderedKeys))

	var wg sync.WaitGroup
	for i := 0; i < workerCount; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			for keyIdx := range jobs {
				key := orderedKeys[keyIdx]
				indices := groups[key]
				firstIdx := indices[0]
				primaryApp := apps[firstIdx]

				update, err := runAppUpdateCheck(primaryApp)
				for _, idx := range indices {
					app := apps[idx]
					results[idx] = managedCheckResult{
						app:    app,
						update: clonePendingManagedUpdateForApp(update, app),
						err:    err,
					}
				}
			}
		}()
	}

	for keyIdx := range orderedKeys {
		jobs <- keyIdx
	}
	close(jobs)

	wg.Wait()
	return results
}

func managedCheckCacheKey(app *models.App, fallbackIdx int) string {
	if app == nil || app.Update == nil {
		return fmt.Sprintf("none:%d", fallbackIdx)
	}

	kind := strings.TrimSpace(string(app.Update.Kind))
	version := normalizeCheckKeyValue(app.Version)
	sha1 := normalizeCheckKeyValue(app.SHA1)

	switch app.Update.Kind {
	case models.UpdateZsync:
		if app.Update.Zsync == nil {
			return fmt.Sprintf("zsync:missing:%s:%s", sha1, kind)
		}
		return fmt.Sprintf("zsync:%s:%s:%s", normalizeCheckKeyValue(app.Update.Zsync.UpdateInfo), normalizeCheckKeyValue(app.Update.Zsync.Transport), sha1)
	case models.UpdateGitHubRelease:
		if app.Update.GitHubRelease == nil {
			return fmt.Sprintf("github:missing:%s", version)
		}
		return fmt.Sprintf("github:%s:%s:%s", normalizeCheckKeyValue(app.Update.GitHubRelease.Repo), normalizeCheckKeyValue(app.Update.GitHubRelease.Asset), version)
	case models.UpdateGitLabRelease:
		if app.Update.GitLabRelease == nil {
			return fmt.Sprintf("gitlab:missing:%s", version)
		}
		return fmt.Sprintf("gitlab:%s:%s:%s", normalizeCheckKeyValue(app.Update.GitLabRelease.Project), normalizeCheckKeyValue(app.Update.GitLabRelease.Asset), version)
	default:
		return fmt.Sprintf("kind:%s:%s:%d", kind, version, fallbackIdx)
	}
}

func normalizeCheckKeyValue(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func clonePendingManagedUpdateForApp(update *pendingManagedUpdate, app *models.App) *pendingManagedUpdate {
	if update == nil {
		return nil
	}

	clone := *update
	clone.App = app
	return &clone
}

func managedCheckWorkerCount(total int) int {
	if total <= 0 {
		return 0
	}

	const maxWorkers = 4
	if total < maxWorkers {
		return total
	}

	return maxWorkers
}

func flushManagedCheckMetadata(updates []repo.CheckMetadataUpdate) error {
	if len(updates) == 0 {
		return nil
	}

	return repo.UpdateCheckMetadataBatch(updates)
}

func persistManagedAppliedApps(apps []*models.App) error {
	if len(apps) == 0 {
		return nil
	}

	if err := addAppsBatch(apps, true); err == nil {
		return nil
	}

	fallbackErrors := make([]string, 0)
	for _, app := range apps {
		if app == nil {
			continue
		}
		if err := addSingleApp(app, true); err != nil {
			fallbackErrors = append(fallbackErrors, fmt.Sprintf("%s: %v", app.ID, err))
		}
	}

	if len(fallbackErrors) > 0 {
		return fmt.Errorf("failed to persist applied updates: %s", strings.Join(fallbackErrors, "; "))
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
		update, err := runZsyncUpdateCheck(app.Update, app.SHA1)
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
	default:
		return nil, fmt.Errorf("unsupported update source kind %q; reconfigure with `aim update set`", app.Update.Kind)
	}
}

func updateCheckMetadata(app *models.App, checked, available bool, latest string) error {
	if app == nil {
		return nil
	}

	lastCheckedAt := util.NowISO()

	if err := repo.UpdateCheckMetadataBatch([]repo.CheckMetadataUpdate{{
		ID:            app.ID,
		Checked:       checked,
		Available:     available,
		Latest:        latest,
		LastCheckedAt: lastCheckedAt,
	}}); err != nil {
		return err
	}

	if checked {
		app.UpdateAvailable = available
		app.LatestVersion = strings.TrimSpace(latest)
	}
	app.LastCheckedAt = lastCheckedAt

	return nil
}

func applyManagedUpdate(ctx context.Context, update pendingManagedUpdate, interactiveProgress bool) (*models.App, error) {
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
	fmt.Printf("  Downloading %s\n", fileName)
	if err := downloadUpdateAsset(ctx, update.URL, downloadPath, interactiveProgress); err != nil {
		return nil, err
	}

	fmt.Println("  Verifying download")
	if err := verifyDownloadedUpdate(downloadPath, update); err != nil {
		return nil, err
	}

	fmt.Println("  Integrating update")
	app, err := core.IntegrateFromLocalFileWithoutCacheRefreshOrPersist(ctx, downloadPath, func(existing, incoming *models.UpdateSource) (bool, error) {
		return false, nil
	})
	if err != nil {
		return nil, err
	}

	return app, nil
}

func verifyDownloadedUpdate(downloadPath string, update pendingManagedUpdate) error {
	expectedSHA256 := strings.ToLower(strings.TrimSpace(update.ExpectedSHA256))
	expectedSHA1 := strings.ToLower(strings.TrimSpace(update.ExpectedSHA1))

	if expectedSHA256 != "" && expectedSHA1 != "" {
		sha256sum, sha1sum, err := util.Sha256AndSha1(downloadPath)
		if err != nil {
			return err
		}
		if strings.ToLower(sha256sum) != expectedSHA256 {
			return fmt.Errorf("downloaded file sha256 mismatch")
		}
		if strings.ToLower(sha1sum) != expectedSHA1 {
			return fmt.Errorf("downloaded file sha1 mismatch")
		}
		return nil
	}

	if expectedSHA256 != "" {
		sum, err := util.Sha256File(downloadPath)
		if err != nil {
			return err
		}
		if strings.ToLower(sum) != expectedSHA256 {
			return fmt.Errorf("downloaded file sha256 mismatch")
		}
	}

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

func buildManagedUpdateMessage(update pendingManagedUpdate, checkOnly bool) string {
	base := update.Label
	if transition := updateVersionTransition(update); transition != "" {
		base = fmt.Sprintf("%s %s", base, transition)
	}

	if !checkOnly {
		return base
	}

	return fmt.Sprintf("%s\nDownload from %s\nThen integrate it with %s", base, update.URL, integrationHint(update.Asset))
}

func updateVersionTransition(update pendingManagedUpdate) string {
	if update.App == nil {
		return ""
	}

	current := displayVersion(update.App.Version)
	latestRaw := strings.TrimSpace(update.Latest)
	if latestRaw == "" {
		return fmt.Sprintf("(current: %s, latest: unknown)", current)
	}

	latest := displayVersion(latestRaw)
	return fmt.Sprintf("(%s -> %s)", current, latest)
}

func displayVersion(value string) string {
	v := strings.TrimSpace(strings.Trim(value, `"'`))
	if v == "" {
		return "unknown"
	}

	lower := strings.ToLower(v)
	if strings.HasPrefix(lower, "version") {
		v = strings.TrimSpace(v[len("version"):])
		v = strings.TrimLeft(v, " :=-")
	}

	v = strings.TrimSpace(v)
	lower = strings.ToLower(v)
	if lower == "" || lower == "n/a" || lower == "na" || lower == "none" || lower == "unknown" || lower == "-" {
		return "unknown"
	}
	if strings.HasPrefix(lower, "v") {
		return v
	}
	return "v" + v
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

func downloadUpdateAsset(ctx context.Context, assetURL, destination string, interactive bool) error {
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

	var (
		total      = resp.ContentLength
		downloaded int64
		buffer     = make([]byte, 32*1024)
		frame      int
		lastDraw   time.Time
	)

	for {
		n, readErr := resp.Body.Read(buffer)
		if n > 0 {
			if _, err := f.Write(buffer[:n]); err != nil {
				return err
			}
			downloaded += int64(n)
		}

		if interactive {
			now := time.Now()
			if now.Sub(lastDraw) >= 120*time.Millisecond || readErr == io.EOF {
				line := buildDownloadProgressLine(downloaded, total, frame)
				fmt.Printf("\r%s", line)
				lastDraw = now
				frame++
			}
		}

		if readErr != nil {
			if readErr == io.EOF {
				break
			}
			return readErr
		}
	}

	if interactive {
		line := buildDownloadProgressLine(downloaded, total, frame)
		fmt.Printf("\r%s\n", line)
	} else {
		fmt.Printf("  Downloaded %s\n", formatByteSize(downloaded))
	}

	return nil
}

func buildDownloadProgressLine(downloaded, total int64, frame int) string {
	if total > 0 {
		percent := float64(downloaded) / float64(total)
		if percent < 0 {
			percent = 0
		}
		if percent > 1 {
			percent = 1
		}

		barWidth := 24
		filled := int(percent * float64(barWidth))
		if filled > barWidth {
			filled = barWidth
		}
		bar := strings.Repeat("#", filled) + strings.Repeat("-", barWidth-filled)
		return fmt.Sprintf("  Downloading [%s] %6.2f%% (%s/%s)", bar, percent*100, formatByteSize(downloaded), formatByteSize(total))
	}

	spinnerFrames := []string{"|", "/", "-", "\\"}
	frameLabel := spinnerFrames[frame%len(spinnerFrames)]
	return fmt.Sprintf("  Downloading %s %s", frameLabel, formatByteSize(downloaded))
}

func formatByteSize(value int64) string {
	if value < 1024 {
		return fmt.Sprintf("%dB", value)
	}

	units := []string{"KB", "MB", "GB", "TB"}
	size := float64(value)
	unit := -1
	for size >= 1024 && unit < len(units)-1 {
		size /= 1024
		unit++
	}

	rounded := strconv.FormatFloat(size, 'f', 1, 64)
	return rounded + units[unit]
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
	_ = cmd
	return isTerminalOutput()
}

func isTerminalOutput() bool {

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
	default:
		return fmt.Sprintf("| %s", update.Kind)
	}
}
