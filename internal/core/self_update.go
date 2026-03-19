package core

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

var (
	upgradeRepoSlug   = "slobbe/appimage-manager"
	upgradeHTTPClient = sharedHTTPClient

	upgradeInstallScriptURL = func(repoSlug string) string {
		return fmt.Sprintf("https://raw.githubusercontent.com/%s/main/scripts/install.sh", repoSlug)
	}
	upgradeLatestReleaseURL = func(repoSlug string) string {
		return fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", repoSlug)
	}
	upgradeShellCommand       = "/bin/sh"
	upgradeRunInstallerScript = runInstallerScript
	upgradeExecutablePath     = os.Executable
	upgradeEvalSymlinks       = filepath.EvalSymlinks
	upgradeRunVersionCommand  = runInstalledVersionCommand
)

type latestReleaseResponse struct {
	TagName string `json:"tag_name"`
}

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
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, upgradeLatestReleaseURL(upgradeRepoSlug), nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := upgradeHTTPClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return "", fmt.Errorf("latest release request failed with status %s", resp.Status)
	}

	var payload latestReleaseResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return "", err
	}

	tag := strings.TrimSpace(payload.TagName)
	if tag == "" {
		return "", fmt.Errorf("latest release response did not include a tag_name")
	}

	return tag, nil
}

func runInstallerScript(ctx context.Context, scriptURL string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, scriptURL, nil)
	if err != nil {
		return err
	}

	resp, err := upgradeHTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return fmt.Errorf("installer script download failed with status %s", resp.Status)
	}

	tempDir, err := os.MkdirTemp("", "aim-upgrade-installer-*")
	if err != nil {
		return err
	}
	defer func() {
		_ = os.RemoveAll(tempDir)
	}()

	scriptPath := filepath.Join(tempDir, "install.sh")
	scriptFile, err := os.OpenFile(scriptPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o700)
	if err != nil {
		return err
	}

	if _, err := io.Copy(scriptFile, resp.Body); err != nil {
		_ = scriptFile.Close()
		return err
	}
	if err := scriptFile.Close(); err != nil {
		return err
	}

	cmd := exec.CommandContext(ctx, upgradeShellCommand, scriptPath)
	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output

	if err := cmd.Run(); err != nil {
		message := strings.TrimSpace(output.String())
		if message == "" {
			return fmt.Errorf("upgrade via installer failed: %w", err)
		}
		return fmt.Errorf("upgrade via installer failed: %w: %s", err, message)
	}

	return nil
}

func resolveInstalledAimPath() (string, error) {
	execPath, err := upgradeExecutablePath()
	if err != nil {
		return "", err
	}

	if resolvedPath, err := upgradeEvalSymlinks(execPath); err == nil && strings.TrimSpace(resolvedPath) != "" {
		execPath = resolvedPath
	}

	return execPath, nil
}

func readInstalledAimVersion(ctx context.Context, binaryPath string) (string, error) {
	output, err := upgradeRunVersionCommand(ctx, binaryPath)
	if err != nil {
		return "", err
	}

	version := strings.TrimSpace(output)
	if version == "" {
		return "", fmt.Errorf("installed version command returned empty output")
	}

	return version, nil
}

func runInstalledVersionCommand(ctx context.Context, binaryPath string) (string, error) {
	cmd := exec.CommandContext(ctx, binaryPath, "--version")
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		message := strings.TrimSpace(stderr.String())
		if message == "" {
			message = strings.TrimSpace(stdout.String())
		}
		if message == "" {
			return "", err
		}
		return "", fmt.Errorf("%w: %s", err, message)
	}

	return stdout.String(), nil
}

func normalizeUpgradeVersion(raw string) string {
	value := strings.TrimSpace(strings.Trim(raw, `"'`))
	if value == "" || strings.EqualFold(value, "dev") {
		return ""
	}
	return NormalizeComparableVersion(value)
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
