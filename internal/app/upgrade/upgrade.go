package upgrade

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	models "github.com/slobbe/appimage-manager/internal/domain"
	"github.com/slobbe/appimage-manager/internal/infra/selfupdate"
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
	upgradeShellCommand       = "/bin/sh"
	upgradeRunInstallerScript = func(ctx context.Context, scriptURL string) error {
		return upgradeInfraClient().RunInstallerScript(ctx, scriptURL, func() (string, error) {
			paths, err := requirePaths()
			if err != nil {
				return "", err
			}
			return paths.TempDir, nil
		})
	}
	upgradeExecutablePath    = os.Executable
	upgradeEvalSymlinks      = filepath.EvalSymlinks
	upgradeRunVersionCommand = selfupdate.RunVersionCommand
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

	currentComparable := normalizeUpgradeVersion(currentRaw)
	latestComparable := normalizeUpgradeVersion(latestRaw)
	if currentComparable == "" || latestComparable == "" || strings.EqualFold(currentRaw, "dev") {
		result.Comparable = false
		return result, nil
	}

	comparison, err := compareUpgradeVersions(currentComparable, latestComparable)
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
	return upgradeInfraClient().FetchLatestReleaseTag(ctx, upgradeLatestReleaseURL(upgradeRepoSlug))
}

func resolveInstalledAimPath() (string, error) {
	return upgradeInfraClient().ResolveInstalledPath()
}

func readInstalledAimVersion(ctx context.Context, binaryPath string) (string, error) {
	return upgradeInfraClient().ReadInstalledVersion(ctx, binaryPath)
}

func upgradeInfraClient() selfupdate.Client {
	return selfupdate.Client{
		HTTPClient:     upgradeHTTPClient,
		ShellCommand:   upgradeShellCommand,
		ExecutablePath: upgradeExecutablePath,
		EvalSymlinks:   upgradeEvalSymlinks,
		VersionCommand: upgradeRunVersionCommand,
	}
}

func normalizeUpgradeVersion(raw string) string {
	value := strings.TrimSpace(strings.Trim(raw, `"'`))
	if value == "" || strings.EqualFold(value, "dev") {
		return ""
	}
	return models.NormalizeComparableVersion(value)
}

func compareUpgradeVersions(left, right string) (int, error) {
	leftVersion, err := parseUpgradeSemver(left)
	if err != nil {
		return 0, err
	}
	rightVersion, err := parseUpgradeSemver(right)
	if err != nil {
		return 0, err
	}

	for i := range leftVersion {
		if leftVersion[i] > rightVersion[i] {
			return 1, nil
		}
		if leftVersion[i] < rightVersion[i] {
			return -1, nil
		}
	}

	return 0, nil
}

func parseUpgradeSemver(version string) ([3]int, error) {
	var parsed [3]int

	normalized := strings.TrimSpace(strings.Trim(version, `"'`))
	if normalized == "" {
		return parsed, fmt.Errorf("invalid version %q", version)
	}

	if idx := strings.IndexAny(normalized, "+-"); idx >= 0 {
		normalized = normalized[:idx]
	}

	parts := strings.Split(normalized, ".")
	if len(parts) < 2 || len(parts) > 3 {
		return parsed, fmt.Errorf("invalid version %q", version)
	}

	for i := range parts {
		value, err := strconv.Atoi(strings.TrimSpace(parts[i]))
		if err != nil || value < 0 {
			return parsed, fmt.Errorf("invalid version %q", version)
		}
		parsed[i] = value
	}

	return parsed, nil
}
