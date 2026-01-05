package models

type OApp struct {
	Name        string `json:"name"`
	Slug        string `json:"slug"`
	Version     string `json:"version"`
	AppImage    string `json:"appimage"`
	Icon        string `json:"icon"`
	Desktop     string `json:"desktop"`
	DesktopLink string `json:"desktopLink"`
	AddedAt     string `json:"addedAt"`
	UpdatedAt   string `json:"updatedAt"`
	SHA256      string `json:"sha256"`
	SHA1        string `json:"sha1"`
	Type        string `json:"type"`
	UpdateInfo  string `json:"updateInfo"`
}

type SourceKind string

const (
	SourceLocalFile     SourceKind = "local_file"
	SourceDirectURL     SourceKind = "direct_url"
	SourceGitHubRelease SourceKind = "github_release"
	SourceZsync         SourceKind = "zsync" // means obtained via zsync/AppImageUpdate
)

type UpdateKind string

const (
	UpdateNone  UpdateKind = "none"
	UpdateZsync UpdateKind = "zsync"
)

type App struct {
	Name    string `json:"name"`    // display name
	ID      string `json:"id"`      // unique app id
	Version string `json:"version"` // current app version

	ExecPath         string `json:"exec_path"`
	IconPath         string `json:"icon_path"`
	DesktopEntryPath string `json:"desktop_entry_path"`
	DesktopEntryLink string `json:"desktop_entry_link"`

	AddedAt   string `json:"added_at"`
	UpdatedAt string `json:"updated_at"`

	SHA256 string `json:"sha256"`
	SHA1   string `json:"sha1"`

	AppImageType string `json:"appimage_type"`

	Source Source `json:"source"`
}

type Source struct {
	Kind SourceKind `json:"kind"`

	// EXACTLY one of these should be set according to Kind
	LocalFile     *LocalFileSource     `json:"local_file,omitempty"`
	DirectURL     *DirectURLSource     `json:"direct_url,omitempty"`
	GitHubRelease *GitHubReleaseSource `json:"github_release,omitempty"`
	Zsync         *ZsyncSource         `json:"zsync,omitempty"` // provenance via zsync

	// optional: how to update from here
	Update *UpdateSource `json:"update,omitempty"`
}

type UpdateSource struct {
	Kind  UpdateKind   `json:"kind"`
	Zsync *ZsyncSource `json:"zsync,omitempty"` // reuse your struct for updater too
}

type LocalFileSource struct {
	IntegratedAt string `json:"integrated_at"`
	OriginalPath string `json:"original_path,omitempty"`
}

type ZsyncSource struct {
	UpdateInfo   string `json:"update_info"`
	UpdateUrl    string `json:"update_url"`
	DownloadedAt string `json:"downloaded_at"`
	Transport    string `json:"transport"` // zsync | gh-releases | bintray | custom
}

type GitHubReleaseSource struct {
	Repo             string `json:"repo"`
	ReleaseID        string `json:"release_id"`
	TagName          string `json:"tag_name"`
	PublishedAt      string `json:"published_at"`
	AssetID          string `json:"asset_id"`
	AssetName        string `json:"asset_name"`
	AssetDownloadURL string `json:"asset_download_url"`
}

type DirectURLSource struct {
	URL          string `json:"url"`
	DownloadedAt string `json:"downloaded_at"`
}
