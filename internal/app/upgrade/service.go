package upgrade

import (
	"context"
	"fmt"
	"strings"

	models "github.com/slobbe/appimage-manager/internal/domain"
)

type ReleaseFinder interface {
	FetchLatestReleaseTag(ctx context.Context, releaseURL string) (string, error)
}

type Service struct {
	RepoSlug         string
	TempDir          string
	SelfUpdater      SelfUpdater
	ReleaseFinder    ReleaseFinder
	InstallScriptURL func(repoSlug string) string
	LatestReleaseURL func(repoSlug string) string
}

func NewService(service Service) Service {
	return service
}

func (service Service) Check(ctx context.Context, currentVersion string) (*AimUpgradeCheckResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	currentRaw := strings.TrimSpace(currentVersion)
	latestRaw, err := service.fetchLatestReleaseTag(ctx)
	if err != nil {
		return nil, err
	}

	result := &AimUpgradeCheckResult{
		CurrentVersion: currentRaw,
		LatestVersion:  latestRaw,
		HasUpdate:      true,
	}

	currentComparable := models.NormalizeUpgradeVersion(currentRaw)
	latestComparable := models.NormalizeUpgradeVersion(latestRaw)
	if currentComparable == "" || latestComparable == "" || strings.EqualFold(currentRaw, "dev") {
		result.Comparable = false
		return result, nil
	}

	comparison, err := models.CompareComparableVersions(currentComparable, latestComparable)
	if err != nil {
		return nil, err
	}

	result.Comparable = true
	result.HasUpdate = comparison < 0
	return result, nil
}

func (service Service) Upgrade(ctx context.Context, currentVersion string) (*InstallerUpgradeResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	result := &InstallerUpgradeResult{PreviousVersion: strings.TrimSpace(currentVersion)}

	updater, err := service.requireSelfUpdater()
	if err != nil {
		return nil, err
	}
	if err := updater.RunInstallerScript(ctx, service.installScriptURL(service.repoSlug()), func() (string, error) {
		return strings.TrimSpace(service.TempDir), nil
	}); err != nil {
		return nil, err
	}

	binaryPath, err := updater.ResolveInstalledPath()
	if err != nil {
		return result, nil
	}

	installedVersion, err := updater.ReadInstalledVersion(ctx, binaryPath)
	if err != nil {
		return result, nil
	}
	result.InstalledVersion = installedVersion
	return result, nil
}

func (service Service) fetchLatestReleaseTag(ctx context.Context) (string, error) {
	finder := service.ReleaseFinder
	if finder == nil {
		updater, err := service.requireSelfUpdater()
		if err != nil {
			return "", err
		}
		finder = updater
	}
	return finder.FetchLatestReleaseTag(ctx, service.latestReleaseURL(service.repoSlug()))
}

func (service Service) requireSelfUpdater() (SelfUpdater, error) {
	if service.SelfUpdater == nil {
		return nil, fmt.Errorf("self updater is not configured")
	}
	return service.SelfUpdater, nil
}

func (service Service) repoSlug() string {
	if strings.TrimSpace(service.RepoSlug) != "" {
		return strings.TrimSpace(service.RepoSlug)
	}
	return "slobbe/appimage-manager"
}

func (service Service) installScriptURL(repoSlug string) string {
	if service.InstallScriptURL != nil {
		return service.InstallScriptURL(repoSlug)
	}
	return fmt.Sprintf("https://raw.githubusercontent.com/%s/main/scripts/install.sh", repoSlug)
}

func (service Service) latestReleaseURL(repoSlug string) string {
	if service.LatestReleaseURL != nil {
		return service.LatestReleaseURL(repoSlug)
	}
	return fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", repoSlug)
}
