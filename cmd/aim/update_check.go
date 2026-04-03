package main

import (
	"fmt"
	"strings"
	"sync"

	util "github.com/slobbe/appimage-manager/internal/helpers"
	repo "github.com/slobbe/appimage-manager/internal/repository"
	models "github.com/slobbe/appimage-manager/internal/types"
	"github.com/spf13/cobra"
)

func printManagedCheckFailures(cmd *cobra.Command, failures []managedCheckFailure) {
	if len(failures) == 0 {
		return
	}

	header := fmt.Sprintf("Failed to check updates for %d app(s)", len(failures))
	writeLogf(cmd, "%s\n", colorize(shouldColorStderr(cmd), "\033[0;31m", header))
	for _, failure := range failures {
		writeLogf(cmd, "  %s\n", strings.TrimSpace(failure.Reason))
	}
	writeLogf(cmd, "  Check network access, provider availability, or rerun a single app with 'aim update <id> --debug'.\n")
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

func runManagedChecksWithCache(cmd *cobra.Command, apps []*models.App, cache *updateCheckCacheFile) ([]managedCheckResult, error) {
	results := make([]managedCheckResult, len(apps))
	if len(apps) == 0 {
		return results, nil
	}
	if cache == nil {
		return runManagedChecks(apps), nil
	}

	toCheck := make([]*models.App, 0, len(apps))
	toCheckIndices := make([]int, 0, len(apps))
	for idx, app := range apps {
		key := managedCheckCacheKey(app, idx)
		if cached, ok := cachedManagedUpdateForApp(cache, app, key); ok {
			results[idx] = managedCheckResult{app: app, update: cached}
			if app != nil {
				logOperationf(cmd, "Reused cached update check for %s", app.ID)
			}
			continue
		}
		toCheck = append(toCheck, app)
		toCheckIndices = append(toCheckIndices, idx)
	}

	fresh := runManagedChecks(toCheck)
	for idx, result := range fresh {
		results[toCheckIndices[idx]] = result
	}

	return results, nil
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
		return wrapWriteError(err)
	}

	if checked {
		app.UpdateAvailable = available
		app.LatestVersion = strings.TrimSpace(latest)
	}
	app.LastCheckedAt = lastCheckedAt

	return nil
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
		latest := strings.TrimSpace(update.NormalizedVersion)
		if !update.Available {
			return &pendingManagedUpdate{
				App:       app,
				Available: false,
				Latest:    latest,
				FromKind:  models.UpdateZsync,
			}, nil
		}
		label := "Update available"
		if update.PreRelease {
			label = "Pre-release update available"
		}
		return &pendingManagedUpdate{
			App:          app,
			URL:          update.DownloadUrl,
			Asset:        update.AssetName,
			Label:        label,
			Available:    true,
			Latest:       latest,
			ExpectedSHA1: strings.TrimSpace(update.RemoteSHA1),
			FromKind:     models.UpdateZsync,
		}, nil
	case models.UpdateGitHubRelease:
		update, err := runGitHubReleaseUpdateCheck(app.Update, app.Version, app.SHA1)
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

		latest := strings.TrimSpace(update.NormalizedVersion)
		if latest == "" {
			latest = strings.TrimSpace(update.TagName)
		}

		if !update.Available {
			return &pendingManagedUpdate{
				App:       app,
				Available: false,
				Latest:    latest,
				FromKind:  models.UpdateGitHubRelease,
			}, nil
		}
		label := "Update available"
		if update.PreRelease {
			label = "Pre-release update available"
		}
		return &pendingManagedUpdate{
			App:          app,
			URL:          update.DownloadUrl,
			Asset:        update.AssetName,
			Label:        label,
			Available:    true,
			Latest:       latest,
			Transport:    update.Transport,
			ZsyncURL:     update.ZsyncURL,
			ExpectedSHA1: strings.TrimSpace(update.ExpectedSHA1),
			FromKind:     models.UpdateGitHubRelease,
		}, nil
	case models.UpdateGitLabRelease:
		update, err := runGitLabReleaseUpdateCheck(app.Update, app.Version, app.SHA1)
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

		latest := strings.TrimSpace(update.NormalizedVersion)
		if latest == "" {
			latest = strings.TrimSpace(update.TagName)
		}
		if !update.Available {
			return &pendingManagedUpdate{
				App:       app,
				Available: false,
				Latest:    latest,
				FromKind:  models.UpdateGitLabRelease,
			}, nil
		}

		return &pendingManagedUpdate{
			App:          app,
			URL:          update.DownloadURL,
			Asset:        update.AssetName,
			Label:        "Update available",
			Available:    true,
			Latest:       latest,
			Transport:    update.Transport,
			ZsyncURL:     update.ZsyncURL,
			ExpectedSHA1: strings.TrimSpace(update.ExpectedSHA1),
			FromKind:     models.UpdateGitLabRelease,
		}, nil
	default:
		return nil, softwareError(fmt.Errorf("unsupported update source for %s: %q", app.ID, app.Update.Kind))
	}
}
