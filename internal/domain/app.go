package domain

import (
	"strings"
	"time"
)

// App is the domain model for an integrated AppImage application.
//
// It contains stable app identity and version information only. Filesystem
// paths are stored as opaque strings; resolving, validating, reading, or writing
// those paths belongs to the app/infra layers.
type SourceKind string

const (
	SourceKindUnknown SourceKind = ""
	SourceKindLocal   SourceKind = "local"
	SourceKindGitHub  SourceKind = "github"
)

type Source struct {
	Kind          SourceKind
	LocalFile     LocalFileSource
	GitHubRelease GitHubReleaseSource
}

type LocalFileSource struct {
	Path         string
	IntegratedAt time.Time
}

type GitHubReleaseSource struct {
	Repo         string
	Tag          string
	Asset        string
	DownloadURL  string
	SizeBytes    int64
	DownloadedAt time.Time
}

type UpdateSourceKind string

const (
	UpdateSourceKindUnknown     UpdateSourceKind = ""
	UpdateSourceKindLocalFile   UpdateSourceKind = "local_file"
	UpdateSourceKindGitHub      UpdateSourceKind = "github"
	UpdateSourceKindZsync       UpdateSourceKind = "zsync"
	UpdateSourceKindUnsupported UpdateSourceKind = "unsupported"
)

type UpdateSource struct {
	Embedded          bool
	Kind              UpdateSourceKind
	Raw               string
	Transport         string
	Repo              string
	Path              string
	Prerelease        bool
	ReleaseTag        string
	AssetPattern      string
	ZsyncAssetPattern string
	URL               string
}

func NewLocalFileUpdateSource(path string) UpdateSource {
	return UpdateSource{
		Embedded: false,
		Kind:     UpdateSourceKindLocalFile,
		Path:     strings.TrimSpace(path),
	}
}

func NewGitHubUpdateSource(repo string, prerelease bool) UpdateSource {
	return UpdateSource{
		Embedded:   false,
		Kind:       UpdateSourceKindGitHub,
		Repo:       strings.TrimSpace(repo),
		Prerelease: prerelease,
	}
}

func NewEmbeddedUpdateSource(raw string) UpdateSource {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return UpdateSource{}
	}

	parts := strings.Split(raw, "|")
	transport := strings.TrimSpace(parts[0])
	source := UpdateSource{
		Embedded:  true,
		Kind:      UpdateSourceKindUnsupported,
		Raw:       raw,
		Transport: transport,
	}

	switch transport {
	case "gh-releases-zsync":
		if len(parts) != 5 {
			return source
		}
		owner := strings.TrimSpace(parts[1])
		repo := strings.TrimSpace(parts[2])
		releaseTag := strings.TrimSpace(parts[3])
		zsyncPattern := strings.TrimSpace(parts[4])
		if owner == "" || repo == "" || releaseTag == "" || zsyncPattern == "" || strings.Contains(owner, "/") || strings.Contains(repo, "/") {
			return source
		}
		source.Kind = UpdateSourceKindGitHub
		source.Repo = owner + "/" + repo
		source.ReleaseTag = releaseTag
		source.ZsyncAssetPattern = zsyncPattern
		source.AssetPattern = strings.TrimSuffix(zsyncPattern, ".zsync")
		return source
	case "zsync":
		if len(parts) != 2 || strings.TrimSpace(parts[1]) == "" {
			return source
		}
		source.Kind = UpdateSourceKindZsync
		source.URL = strings.TrimSpace(parts[1])
		return source
	case "pling-v1-zsync", "bintray-zsync":
		return source
	default:
		return source
	}
}

func NewLocalSource(path string, integratedAt time.Time) Source {
	return Source{
		Kind: SourceKindLocal,
		LocalFile: LocalFileSource{
			Path:         strings.TrimSpace(path),
			IntegratedAt: normalizeSourceTime(integratedAt),
		},
	}
}

func NewGitHubReleaseSource(repo string, tag string, asset string, downloadURL string, sizeBytes int64, downloadedAt time.Time) Source {
	return Source{
		Kind: SourceKindGitHub,
		GitHubRelease: GitHubReleaseSource{
			Repo:         strings.TrimSpace(repo),
			Tag:          strings.TrimSpace(tag),
			Asset:        strings.TrimSpace(asset),
			DownloadURL:  strings.TrimSpace(downloadURL),
			SizeBytes:    sizeBytes,
			DownloadedAt: normalizeSourceTime(downloadedAt),
		},
	}
}

func normalizeSourceTime(value time.Time) time.Time {
	if value.IsZero() {
		return time.Time{}
	}

	return value.UTC().Truncate(0)
}

type App struct {
	ID               string
	Name             string
	Version          Version
	AppImagePath     string
	DesktopEntryPath string
	IconPath         string
	Source           Source
	UpdateSource     UpdateSource
}

// NewApp creates an App and derives its ID from the name when no explicit ID is
// provided.
func NewApp(input AppInput) App {
	name := strings.TrimSpace(input.Name)
	id := strings.TrimSpace(input.ID)
	if id == "" {
		id = Slugify(name)
	} else {
		id = Slugify(id)
	}

	return App{
		ID:               id,
		Name:             name,
		Version:          input.Version,
		AppImagePath:     strings.TrimSpace(input.AppImagePath),
		DesktopEntryPath: strings.TrimSpace(input.DesktopEntryPath),
		IconPath:         strings.TrimSpace(input.IconPath),
		Source:           input.Source,
		UpdateSource:     input.UpdateSource,
	}
}

// NewAppFromDesktopEntry creates an App from parsed desktop metadata and
// installation/use-case input.
func NewAppFromDesktopEntry(entry DesktopEntry, input AppInput) App {
	input.Name = entry.Name
	input.Version = entry.Version
	return NewApp(input)
}

// AppInput contains the values needed to construct an App.
type AppInput struct {
	ID               string
	Name             string
	Version          Version
	AppImagePath     string
	DesktopEntryPath string
	IconPath         string
	Source           Source
	UpdateSource     UpdateSource
}

// HasUpdate reports whether candidate is newer than the app's current version.
func (a App) HasUpdate(candidate Version) bool {
	if a.Version.IsZero() || candidate.IsZero() {
		return false
	}

	return CompareVersions(candidate.String(), a.Version.String()) > 0
}
