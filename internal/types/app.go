package models

type SourceKind string

const (
	SourceLocalFile SourceKind = "local_file"
)

type UpdateKind string

const (
	UpdateNone          UpdateKind = "none"
	UpdateZsync         UpdateKind = "zsync"
	UpdateGitHubRelease UpdateKind = "github_release"
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

	Source Source        `json:"source"`
	Update *UpdateSource `json:"update,omitempty"` // optional: how to update from here
}

type Source struct {
	Kind SourceKind `json:"kind"`

	// EXACTLY one of these should be set according to Kind
	LocalFile *LocalFileSource `json:"local_file,omitempty"`
}

type LocalFileSource struct {
	IntegratedAt string `json:"integrated_at"`
	OriginalPath string `json:"original_path,omitempty"`
}

type UpdateSource struct {
	Kind          UpdateKind                 `json:"kind"`
	Zsync         *ZsyncUpdateSource         `json:"zsync,omitempty"`
	GitHubRelease *GitHubReleaseUpdateSource `json:"github_release,omitempty"`
}

type ZsyncUpdateSource struct {
	UpdateInfo string `json:"update_info"`
	Transport  string `json:"transport"` // zsync | gh-releases
}

type GitHubReleaseUpdateSource struct {
	Repo        string `json:"repo"`
	Asset       string `json:"asset"`
	ReleaseKind string `json:"release_kind,omitempty"`
}
