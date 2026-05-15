package upgrade

import (
	"context"
	"fmt"
	"strings"

	models "github.com/slobbe/appimage-manager/internal/domain"
)

var (
	upgradeRepoSlug   = "slobbe/appimage-manager"
	upgradeHTTPClient = SharedHTTPClient()

	upgradeInstallScriptURL = func(repoSlug string) string {
		return fmt.Sprintf("https://raw.githubusercontent.com/%s/main/scripts/install.sh", repoSlug)
	}
	upgradeLatestReleaseURL = func(repoSlug string) string {
		return fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", repoSlug)
	}
	upgradeRunInstallerScript = func(ctx context.Context, scriptURL string) error {
		updater, err := requireSelfUpdater()
		if err != nil {
			return err
		}
		return updater.RunInstallerScript(ctx, scriptURL, func() (string, error) {
			paths, err := requirePaths()
			if err != nil {
				return "", err
			}
			return paths.TempDir, nil
		})
	}
)

type AimUpgradeCheckResult struct {
	CurrentVersion string
	LatestVersion  string
	HasUpdate      bool
	Comparable     bool
}

type InstallerUpgradeResult struct {
	PreviousVersion  string
	InstalledVersion string
}

func CheckForAimUpgrade(ctx context.Context, currentVersion string) (*AimUpgradeCheckResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	currentRaw := strings.TrimSpace(currentVersion)
	latestRaw, err := fetchLatestReleaseTag(ctx)
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

func UpgradeViaInstaller(ctx context.Context, currentVersion string) (*InstallerUpgradeResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	result := &InstallerUpgradeResult{
		PreviousVersion: strings.TrimSpace(currentVersion),
	}

	if err := upgradeRunInstallerScript(ctx, upgradeInstallScriptURL(upgradeRepoSlug)); err != nil {
		return nil, err
	}

	binaryPath, err := resolveInstalledAimPath()
	if err != nil {
		return result, nil
	}

	installedVersion, err := readInstalledAimVersion(ctx, binaryPath)
	if err != nil {
		return result, nil
	}
	result.InstalledVersion = installedVersion

	return result, nil
}

func fetchLatestReleaseTag(ctx context.Context) (string, error) {
	updater, err := requireSelfUpdater()
	if err != nil {
		return "", err
	}
	return updater.FetchLatestReleaseTag(ctx, upgradeLatestReleaseURL(upgradeRepoSlug))
}

func resolveInstalledAimPath() (string, error) {
	updater, err := requireSelfUpdater()
	if err != nil {
		return "", err
	}
	return updater.ResolveInstalledPath()
}

func readInstalledAimVersion(ctx context.Context, binaryPath string) (string, error) {
	updater, err := requireSelfUpdater()
	if err != nil {
		return "", err
	}
	return updater.ReadInstalledVersion(ctx, binaryPath)
}
