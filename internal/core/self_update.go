package core

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

type SelfUpgradeResult struct {
	Updated        bool
	CurrentVersion string
	LatestVersion  string
}

type selfUpdateReleaseResponse struct {
	TagName string `json:"tag_name"`
	Assets  []struct {
		Name               string `json:"name"`
		BrowserDownloadURL string `json:"browser_download_url"`
	} `json:"assets"`
}

var (
	selfUpdateRepoSlug = "slobbe/appimage-manager"

	selfUpdateHTTPClient = http.DefaultClient
	selfUpdateGOARCH     = runtime.GOARCH

	selfUpdateExecutablePath = os.Executable
	selfUpdateInstall        = installDownloadedRelease

	selfUpdateLatestReleaseURL = func(repoSlug string) string {
		return fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", repoSlug)
	}
)

func SelfUpgrade(ctx context.Context, currentVersion string) (*SelfUpgradeResult, error) {
	current := normalizeSelfUpdateVersion(currentVersion)
	if current == "" || current == "dev" {
		return nil, fmt.Errorf("self-upgrade is unavailable for development builds")
	}

	assetName, err := releaseAssetNameForArch(selfUpdateGOARCH)
	if err != nil {
		return nil, err
	}

	release, err := fetchLatestStableRelease(ctx)
	if err != nil {
		return nil, err
	}

	latest := normalizeSelfUpdateVersion(release.TagName)
	if latest == "" {
		return nil, fmt.Errorf("invalid latest release tag %q", release.TagName)
	}

	comparison, err := compareSemanticVersions(latest, current)
	if err != nil {
		return nil, err
	}

	if comparison <= 0 {
		return &SelfUpgradeResult{
			Updated:        false,
			CurrentVersion: current,
			LatestVersion:  latest,
		}, nil
	}

	assetURL, err := findReleaseAssetURL(release.Assets, assetName)
	if err != nil {
		return nil, err
	}

	if err := selfUpdateInstall(ctx, assetURL, latest, selfUpdateGOARCH); err != nil {
		return nil, err
	}

	return &SelfUpgradeResult{
		Updated:        true,
		CurrentVersion: current,
		LatestVersion:  latest,
	}, nil
}

func fetchLatestStableRelease(ctx context.Context) (*selfUpdateReleaseResponse, error) {
	releaseURL := selfUpdateLatestReleaseURL(selfUpdateRepoSlug)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, releaseURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := selfUpdateHTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, fmt.Errorf("github api returned status %s", resp.Status)
	}

	var payload selfUpdateReleaseResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}

	return &payload, nil
}

func releaseAssetNameForArch(arch string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(arch)) {
	case "amd64":
		return "aim-linux-amd64.tar.gz", nil
	case "arm64":
		return "aim-linux-arm64.tar.gz", nil
	default:
		return "", fmt.Errorf("unsupported architecture: %s", arch)
	}
}

func findReleaseAssetURL(assets []struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}, assetName string) (string, error) {
	for _, asset := range assets {
		if strings.TrimSpace(asset.Name) == assetName && strings.TrimSpace(asset.BrowserDownloadURL) != "" {
			return asset.BrowserDownloadURL, nil
		}
	}

	return "", fmt.Errorf("release asset %q not found", assetName)
}

func installDownloadedRelease(ctx context.Context, assetURL, releaseVersion, arch string) error {
	execPath, err := selfUpdateExecutablePath()
	if err != nil {
		return fmt.Errorf("failed to resolve executable path: %w", err)
	}

	if resolved, err := filepath.EvalSymlinks(execPath); err == nil {
		execPath = resolved
	}

	tempDir, err := os.MkdirTemp("", "aim-self-update-*")
	if err != nil {
		return err
	}
	defer func() {
		_ = os.RemoveAll(tempDir)
	}()

	archivePath := filepath.Join(tempDir, "aim-release.tar.gz")
	if err := downloadReleaseArchive(ctx, assetURL, archivePath); err != nil {
		return err
	}

	extractedBinaryPath := filepath.Join(tempDir, "aim-new")
	if err := extractReleaseBinary(archivePath, extractedBinaryPath, arch); err != nil {
		return err
	}

	if err := os.Chmod(extractedBinaryPath, 0o755); err != nil {
		return err
	}

	if err := replaceExecutableBinary(extractedBinaryPath, execPath); err != nil {
		return fmt.Errorf("failed to install v%s: %w", releaseVersion, err)
	}

	return nil
}

func downloadReleaseArchive(ctx context.Context, assetURL, targetPath string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, assetURL, nil)
	if err != nil {
		return err
	}

	resp, err := selfUpdateHTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return fmt.Errorf("download failed with status %s", resp.Status)
	}

	f, err := os.OpenFile(targetPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = io.Copy(f, resp.Body)
	return err
}

func extractReleaseBinary(archivePath, outputPath, arch string) error {
	f, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer f.Close()

	gzReader, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gzReader.Close()

	tarReader := tar.NewReader(gzReader)

	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		if header.Typeflag != tar.TypeReg && header.Typeflag != tar.TypeRegA {
			continue
		}

		name := filepath.Clean(header.Name)
		if filepath.IsAbs(name) || strings.HasPrefix(name, "..") {
			continue
		}

		baseName := filepath.Base(name)
		if baseName != name {
			continue
		}

		if !isReleaseBinaryName(baseName, arch) {
			continue
		}

		out, err := os.OpenFile(outputPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
		if err != nil {
			return err
		}

		if _, err := io.Copy(out, tarReader); err != nil {
			_ = out.Close()
			return err
		}

		if err := out.Close(); err != nil {
			return err
		}

		return nil
	}

	return fmt.Errorf("no release binary found in archive")
}

func isReleaseBinaryName(name, arch string) bool {
	arch = strings.ToLower(strings.TrimSpace(arch))
	if arch == "" {
		return false
	}
	return strings.HasPrefix(name, "aim-") && strings.HasSuffix(name, "-linux-"+arch)
}

func replaceExecutableBinary(sourcePath, targetPath string) error {
	stagePath := filepath.Join(filepath.Dir(targetPath), fmt.Sprintf(".aim-upgrade-%d.tmp", time.Now().UnixNano()))

	if err := copyFileWithMode(sourcePath, stagePath, 0o755); err != nil {
		return err
	}

	if err := os.Rename(stagePath, targetPath); err != nil {
		_ = os.Remove(stagePath)
		return err
	}

	return nil
}

func copyFileWithMode(src, dst string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}

	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}

	return out.Close()
}

func normalizeSelfUpdateVersion(version string) string {
	v := strings.TrimSpace(strings.ToLower(version))
	if v == "dev" {
		return v
	}
	v = strings.TrimPrefix(v, "version")
	v = strings.TrimSpace(v)
	v = strings.TrimPrefix(v, "v")
	return strings.TrimSpace(v)
}

func compareSemanticVersions(left, right string) (int, error) {
	lv, err := parseSemver(left)
	if err != nil {
		return 0, err
	}
	rv, err := parseSemver(right)
	if err != nil {
		return 0, err
	}

	if lv[0] != rv[0] {
		if lv[0] > rv[0] {
			return 1, nil
		}
		return -1, nil
	}
	if lv[1] != rv[1] {
		if lv[1] > rv[1] {
			return 1, nil
		}
		return -1, nil
	}
	if lv[2] != rv[2] {
		if lv[2] > rv[2] {
			return 1, nil
		}
		return -1, nil
	}

	return 0, nil
}

func parseSemver(version string) ([3]int, error) {
	v := normalizeSelfUpdateVersion(version)
	if v == "" || v == "dev" {
		return [3]int{}, fmt.Errorf("invalid version %q", version)
	}

	if idx := strings.IndexAny(v, "+-"); idx >= 0 {
		v = v[:idx]
	}

	parts := strings.Split(v, ".")
	if len(parts) == 0 || len(parts) > 3 {
		return [3]int{}, fmt.Errorf("invalid version %q", version)
	}

	parsed := [3]int{}
	for i := range parts {
		part := strings.TrimSpace(parts[i])
		if part == "" {
			return [3]int{}, fmt.Errorf("invalid version %q", version)
		}

		n, err := strconv.Atoi(part)
		if err != nil || n < 0 {
			return [3]int{}, fmt.Errorf("invalid version %q", version)
		}
		parsed[i] = n
	}

	return parsed, nil
}
