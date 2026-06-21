package output

import (
	"time"

	"github.com/slobbe/appimage-manager/internal/app"
)

type InfoJSON struct {
	ID           string           `json:"id,omitempty"`
	Name         string           `json:"name"`
	Version      string           `json:"version"`
	ExecPath     string           `json:"exec_path"`
	Installed    bool             `json:"installed"`
	TargetKind   string           `json:"target_kind"`
	Source       SourceJSON       `json:"source"`
	UpdateSource UpdateSourceJSON `json:"update_source"`
}

type UpdateSourceJSON struct {
	Embedded          bool   `json:"embedded"`
	Kind              string `json:"kind"`
	Raw               string `json:"raw,omitempty"`
	Transport         string `json:"transport,omitempty"`
	Repo              string `json:"repo,omitempty"`
	Path              string `json:"path,omitempty"`
	Prerelease        bool   `json:"prerelease,omitempty"`
	ReleaseTag        string `json:"release_tag,omitempty"`
	AssetPattern      string `json:"asset_pattern,omitempty"`
	ZsyncAssetPattern string `json:"zsync_asset_pattern,omitempty"`
	URL               string `json:"url,omitempty"`
}

type SourceJSON struct {
	Kind          string                   `json:"kind"`
	LocalFile     *LocalFileSourceJSON     `json:"local_file,omitempty"`
	GitHubRelease *GitHubReleaseSourceJSON `json:"github_release,omitempty"`
}

type LocalFileSourceJSON struct {
	Path         string `json:"path"`
	IntegratedAt string `json:"integrated_at,omitempty"`
}

type GitHubReleaseSourceJSON struct {
	Repo         string `json:"repo"`
	Tag          string `json:"tag,omitempty"`
	Asset        string `json:"asset,omitempty"`
	DownloadURL  string `json:"download_url,omitempty"`
	SizeBytes    int64  `json:"size_bytes,omitempty"`
	DownloadedAt string `json:"downloaded_at,omitempty"`
}

func InfoResultJSON(info app.InfoResult) InfoJSON {
	return InfoJSON{
		ID:           info.ID,
		Name:         info.Name,
		Version:      info.Version,
		ExecPath:     info.ExecPath,
		Installed:    info.Installed,
		TargetKind:   info.TargetKind,
		Source:       sourceJSON(info),
		UpdateSource: updateSourceJSON(info),
	}
}

func updateSourceJSON(info app.InfoResult) UpdateSourceJSON {
	source := info.UpdateSource
	return UpdateSourceJSON{
		Embedded:          source.Embedded,
		Kind:              string(source.Kind),
		Raw:               source.Raw,
		Transport:         source.Transport,
		Repo:              source.Repo,
		Path:              source.Path,
		Prerelease:        source.Prerelease,
		ReleaseTag:        source.ReleaseTag,
		AssetPattern:      source.AssetPattern,
		ZsyncAssetPattern: source.ZsyncAssetPattern,
		URL:               source.URL,
	}
}

func sourceJSON(info app.InfoResult) SourceJSON {
	source := info.Source
	result := SourceJSON{Kind: string(source.Kind)}
	switch string(source.Kind) {
	case "local":
		result.LocalFile = &LocalFileSourceJSON{
			Path:         source.LocalFile.Path,
			IntegratedAt: FormatSourceTime(source.LocalFile.IntegratedAt),
		}
	case "github":
		result.GitHubRelease = &GitHubReleaseSourceJSON{
			Repo:         source.GitHubRelease.Repo,
			Tag:          source.GitHubRelease.Tag,
			Asset:        source.GitHubRelease.Asset,
			DownloadURL:  source.GitHubRelease.DownloadURL,
			SizeBytes:    source.GitHubRelease.SizeBytes,
			DownloadedAt: FormatSourceTime(source.GitHubRelease.DownloadedAt),
		}
	}

	return result
}

func FormatSourceTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}

	return value.UTC().Format(time.RFC3339)
}
