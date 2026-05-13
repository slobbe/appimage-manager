package app

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"strings"

	models "github.com/slobbe/appimage-manager/internal/domain"
)

func ResolveManagedAppID(appName, upstreamID, hashSeed string, incoming *models.App) (string, *models.App, error) {
	store, err := requireStore()
	if err != nil {
		return "", nil, err
	}

	return resolveManagedAppID(store, appName, upstreamID, hashSeed, incoming)
}

func resolveManagedAppID(store AppStore, appName, upstreamID, hashSeed string, incoming *models.App) (string, *models.App, error) {
	candidates := managedIDCandidates(appName, upstreamID, hashSeed)
	if len(candidates) == 0 {
		return "", nil, fmt.Errorf("managed app id cannot be empty")
	}

	allApps, err := store.GetAllApps()
	if err != nil {
		return "", nil, err
	}

	var equivalentApp *models.App
	for _, existing := range allApps {
		if existing == nil || strings.TrimSpace(existing.ID) == "" {
			continue
		}
		if appsShareManagedIdentity(existing, incoming) {
			if equivalentApp != nil && strings.TrimSpace(equivalentApp.ID) != strings.TrimSpace(existing.ID) {
				equivalentApp = nil
				break
			}
			equivalentApp = existing
		}
	}

	for _, candidate := range candidates {
		existing := allApps[candidate]
		if existing == nil {
			if equivalentApp != nil && strings.TrimSpace(equivalentApp.ID) != candidate {
				return candidate, equivalentApp, nil
			}
			return candidate, nil, nil
		}
		if appsShareManagedIdentity(existing, incoming) {
			return candidate, nil, nil
		}
	}

	fallback := candidates[len(candidates)-1]
	if equivalentApp != nil && strings.TrimSpace(equivalentApp.ID) != fallback {
		return fallback, equivalentApp, nil
	}
	return fallback, nil, nil
}

func managedIDCandidates(appName, upstreamID, hashSeed string) []string {
	upstreamID = strings.TrimSpace(upstreamID)
	base := models.Slugify(appName)
	if base == "" {
		base = upstreamID
	}
	base = strings.TrimSpace(base)
	if base == "" {
		return nil
	}

	candidates := []string{base}
	if upstreamID != "" {
		withUpstream := base + "-" + upstreamID
		if !containsString(candidates, withUpstream) {
			candidates = append(candidates, withUpstream)
		}

		withHash := withUpstream + "-" + shortIdentityHash(upstreamID, hashSeed)
		if !containsString(candidates, withHash) {
			candidates = append(candidates, withHash)
		}
	}

	return candidates
}

func shortIdentityHash(parts ...string) string {
	seed := strings.Join(parts, "\x00")
	sum := sha1.Sum([]byte(seed))
	return hex.EncodeToString(sum[:])[:6]
}

func containsString(values []string, needle string) bool {
	for _, value := range values {
		if value == needle {
			return true
		}
	}
	return false
}

func FindEquivalentManagedApp(incoming *models.App) (*models.App, error) {
	if incoming == nil {
		return nil, nil
	}

	store, err := requireStore()
	if err != nil {
		return nil, err
	}

	allApps, err := store.GetAllApps()
	if err != nil {
		return nil, err
	}

	var match *models.App
	for _, existing := range allApps {
		if existing == nil {
			continue
		}
		if strings.TrimSpace(existing.ID) == "" || strings.TrimSpace(existing.ID) == strings.TrimSpace(incoming.ID) {
			continue
		}
		if !appsShareManagedIdentity(existing, incoming) {
			continue
		}
		if match != nil {
			return nil, nil
		}
		match = existing
	}

	return match, nil
}

func appsShareManagedIdentity(a, b *models.App) bool {
	if a == nil || b == nil {
		return false
	}

	if UpdateSourcesEqual(a.Update, b.Update) {
		return true
	}

	return SourcesEqual(&a.Source, &b.Source)
}

func UpdateSourcesEqual(a, b *models.UpdateSource) bool {
	if a == nil || b == nil {
		return false
	}
	if a.Kind != b.Kind {
		return false
	}

	switch a.Kind {
	case models.UpdateNone:
		return false
	case models.UpdateGitHubRelease:
		return a.GitHubRelease != nil && b.GitHubRelease != nil &&
			strings.TrimSpace(a.GitHubRelease.Repo) == strings.TrimSpace(b.GitHubRelease.Repo) &&
			strings.TrimSpace(a.GitHubRelease.Asset) == strings.TrimSpace(b.GitHubRelease.Asset)
	case models.UpdateZsync:
		return a.Zsync != nil && b.Zsync != nil &&
			strings.TrimSpace(a.Zsync.UpdateInfo) == strings.TrimSpace(b.Zsync.UpdateInfo) &&
			strings.TrimSpace(a.Zsync.Transport) == strings.TrimSpace(b.Zsync.Transport)
	default:
		return false
	}
}

func SourcesEqual(a, b *models.Source) bool {
	if a == nil || b == nil {
		return false
	}
	if a.Kind != b.Kind {
		return false
	}

	switch a.Kind {
	case models.SourceLocalFile:
		return a.LocalFile != nil && b.LocalFile != nil &&
			normalizeLocalSourcePath(a.LocalFile.OriginalPath) != "" &&
			normalizeLocalSourcePath(a.LocalFile.OriginalPath) == normalizeLocalSourcePath(b.LocalFile.OriginalPath)
	case models.SourceDirectURL:
		return a.DirectURL != nil && b.DirectURL != nil &&
			strings.TrimSpace(a.DirectURL.URL) != "" &&
			strings.TrimSpace(a.DirectURL.URL) == strings.TrimSpace(b.DirectURL.URL)
	case models.SourceGitHubRelease:
		return a.GitHubRelease != nil && b.GitHubRelease != nil &&
			strings.TrimSpace(a.GitHubRelease.Repo) == strings.TrimSpace(b.GitHubRelease.Repo) &&
			strings.TrimSpace(a.GitHubRelease.Asset) == strings.TrimSpace(b.GitHubRelease.Asset)
	default:
		return false
	}
}

func normalizeLocalSourcePath(path string) string {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return ""
	}
	return filepath.Clean(trimmed)
}
