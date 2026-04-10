package core

import (
	"path/filepath"
	"strings"

	repo "github.com/slobbe/appimage-manager/internal/repository"
	models "github.com/slobbe/appimage-manager/internal/types"
)

func FindEquivalentManagedApp(incoming *models.App) (*models.App, error) {
	if incoming == nil {
		return nil, nil
	}

	allApps, err := repo.GetAllApps()
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
		return true
	case models.UpdateGitHubRelease:
		return a.GitHubRelease != nil && b.GitHubRelease != nil &&
			strings.TrimSpace(a.GitHubRelease.Repo) == strings.TrimSpace(b.GitHubRelease.Repo) &&
			strings.TrimSpace(a.GitHubRelease.Asset) == strings.TrimSpace(b.GitHubRelease.Asset)
	case models.UpdateGitLabRelease:
		return a.GitLabRelease != nil && b.GitLabRelease != nil &&
			strings.TrimSpace(a.GitLabRelease.Project) == strings.TrimSpace(b.GitLabRelease.Project) &&
			strings.TrimSpace(a.GitLabRelease.Asset) == strings.TrimSpace(b.GitLabRelease.Asset)
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
	case models.SourceGitLabRelease:
		return a.GitLabRelease != nil && b.GitLabRelease != nil &&
			strings.TrimSpace(a.GitLabRelease.Project) == strings.TrimSpace(b.GitLabRelease.Project) &&
			strings.TrimSpace(a.GitLabRelease.Asset) == strings.TrimSpace(b.GitLabRelease.Asset)
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
