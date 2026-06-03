package update

import (
	"fmt"
	"strings"
	"sync"

	models "github.com/slobbe/appimage-manager/internal/domain"
)

type ManagedCheckResult struct {
	App    *models.App
	Update *ManagedUpdate
	Error  error
}

type ManagedCheckFunc func(app *models.App) (*ManagedUpdate, error)

func CheckManagedUpdates(apps []*models.App, check ManagedCheckFunc) []ManagedCheckResult {
	results := make([]ManagedCheckResult, len(apps))
	if len(apps) == 0 {
		return results
	}
	if check == nil {
		checker := NewManagedUpdateChecker(ManagedUpdateChecker{})
		check = checker.Check
	}

	groups := make(map[string][]int, len(apps))
	orderedKeys := make([]string, 0, len(apps))
	for idx, app := range apps {
		key := ManagedCheckCacheKey(app, idx)
		if _, exists := groups[key]; !exists {
			orderedKeys = append(orderedKeys, key)
		}
		groups[key] = append(groups[key], idx)
	}

	jobs := make(chan int, len(orderedKeys))
	workerCount := ManagedCheckWorkerCount(len(orderedKeys))

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

				update, err := check(primaryApp)
				for _, idx := range indices {
					app := apps[idx]
					results[idx] = ManagedCheckResult{
						App:    app,
						Update: CloneManagedUpdateForApp(update, app),
						Error:  err,
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

func ManagedCheckCacheKey(app *models.App, fallbackIdx int) string {
	if app == nil || app.Update == nil {
		return fmt.Sprintf("none:%d", fallbackIdx)
	}

	kind := strings.TrimSpace(string(app.Update.Kind))
	version := normalizeManagedCheckKeyValue(app.Version)
	sha1 := normalizeManagedCheckKeyValue(app.SHA1)

	switch app.Update.Kind {
	case models.UpdateZsync:
		if app.Update.Zsync == nil {
			return fmt.Sprintf("zsync:missing:%s:%s", sha1, kind)
		}
		return fmt.Sprintf("zsync:%s:%s:%s", normalizeManagedCheckKeyValue(app.Update.Zsync.UpdateInfo), normalizeManagedCheckKeyValue(app.Update.Zsync.Transport), sha1)
	case models.UpdateGitHubRelease:
		if app.Update.GitHubRelease == nil {
			return fmt.Sprintf("github:missing:%s", version)
		}
		return fmt.Sprintf("github:%s:%s:%s", normalizeManagedCheckKeyValue(app.Update.GitHubRelease.Repo), normalizeManagedCheckKeyValue(app.Update.GitHubRelease.Asset), version)
	default:
		return fmt.Sprintf("kind:%s:%s:%d", kind, version, fallbackIdx)
	}
}

func CloneManagedUpdateForApp(update *ManagedUpdate, app *models.App) *ManagedUpdate {
	if update == nil {
		return nil
	}
	clone := *update
	clone.App = app
	return &clone
}

func ManagedCheckWorkerCount(total int) int {
	if total <= 0 {
		return 0
	}
	const maxWorkers = 4
	if total < maxWorkers {
		return total
	}
	return maxWorkers
}

func normalizeManagedCheckKeyValue(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}
