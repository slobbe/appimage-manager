package update

import (
	"fmt"
	"strings"

	models "github.com/slobbe/appimage-manager/internal/domain"
)

type ManagedUpdateChecker struct {
	ZsyncMetadataFetcher  ZsyncMetadataFetcher
	GitHubReleaseResolver GitHubReleaseResolver
	ZsyncCheck            func(update *models.UpdateSource, localSHA1 string) (*UpdateData, error)
	GitHubReleaseCheck    func(update *models.UpdateSource, currentVersion, localSHA1 string) (*GitHubReleaseUpdate, error)
}

func NewManagedUpdateChecker(checker ManagedUpdateChecker) ManagedUpdateChecker {
	return checker
}

func (checker ManagedUpdateChecker) Check(app *models.App) (*ManagedUpdate, error) {
	if app == nil || app.Update == nil || app.Update.Kind == models.UpdateNone {
		return nil, nil
	}

	switch app.Update.Kind {
	case models.UpdateZsync:
		return checker.checkZsync(app)
	case models.UpdateGitHubRelease:
		return checker.checkGitHubRelease(app)
	default:
		return nil, fmt.Errorf("unsupported update source for %s: %q", app.ID, app.Update.Kind)
	}
}

func (checker ManagedUpdateChecker) checkZsync(app *models.App) (*ManagedUpdate, error) {
	check := checker.ZsyncCheck
	if check == nil {
		check = func(update *models.UpdateSource, localSHA1 string) (*UpdateData, error) {
			return ZsyncUpdateCheckWithFetcher(update, localSHA1, checker.ZsyncMetadataFetcher)
		}
	}
	update, err := check(app.Update, app.SHA1)
	if err != nil {
		return nil, err
	}
	if update == nil {
		return &ManagedUpdate{App: app, Available: false, Latest: "", FromKind: models.UpdateZsync}, nil
	}

	latest := strings.TrimSpace(update.NormalizedVersion)
	if !update.Available {
		return &ManagedUpdate{App: app, Available: false, Latest: latest, FromKind: models.UpdateZsync}, nil
	}
	return &ManagedUpdate{
		App:          app,
		URL:          update.DownloadUrl,
		Asset:        update.AssetName,
		Label:        models.UpdateAvailabilityLabel(update.PreRelease),
		Available:    true,
		Latest:       latest,
		ExpectedSHA1: strings.TrimSpace(update.RemoteSHA1),
		FromKind:     models.UpdateZsync,
	}, nil
}

func (checker ManagedUpdateChecker) checkGitHubRelease(app *models.App) (*ManagedUpdate, error) {
	check := checker.GitHubReleaseCheck
	if check == nil {
		check = func(update *models.UpdateSource, currentVersion, localSHA1 string) (*GitHubReleaseUpdate, error) {
			return GitHubReleaseUpdateCheckWithResolver(update, currentVersion, localSHA1, checker.GitHubReleaseResolver, checker.ZsyncMetadataFetcher)
		}
	}
	update, err := check(app.Update, app.Version, app.SHA1)
	if err != nil {
		return nil, err
	}
	if update == nil {
		return &ManagedUpdate{App: app, Available: false, Latest: "", FromKind: models.UpdateGitHubRelease}, nil
	}

	latest := strings.TrimSpace(update.NormalizedVersion)
	if latest == "" {
		latest = strings.TrimSpace(update.TagName)
	}
	if !update.Available {
		return &ManagedUpdate{App: app, Available: false, Latest: latest, FromKind: models.UpdateGitHubRelease}, nil
	}
	return &ManagedUpdate{
		App:          app,
		URL:          update.DownloadUrl,
		Asset:        update.AssetName,
		Label:        models.UpdateAvailabilityLabel(update.PreRelease),
		Available:    true,
		Latest:       latest,
		Transport:    update.Transport,
		ZsyncURL:     update.ZsyncURL,
		ExpectedSHA1: strings.TrimSpace(update.ExpectedSHA1),
		FromKind:     models.UpdateGitHubRelease,
	}, nil
}
