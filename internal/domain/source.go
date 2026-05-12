package domain

type SourceKind string

const (
	SourceLocalFile     SourceKind = "local_file"
	SourceDirectURL     SourceKind = "direct_url"
	SourceGitHubRelease SourceKind = "github_release"
)

type Source struct {
	Kind SourceKind `json:"kind"`

	// EXACTLY one of these should be set according to Kind
	LocalFile     *LocalFileSource     `json:"local_file,omitempty"`
	DirectURL     *DirectURLSource     `json:"direct_url,omitempty"`
	GitHubRelease *GitHubReleaseSource `json:"github_release,omitempty"`
}

type LocalFileSource struct {
	IntegratedAt string `json:"integrated_at"`
	OriginalPath string `json:"original_path,omitempty"`
}

type DirectURLSource struct {
	URL          string `json:"url"`
	SHA256       string `json:"sha256,omitempty"`
	DownloadedAt string `json:"downloaded_at"`
}

type GitHubReleaseSource struct {
	Repo         string `json:"repo"`
	Asset        string `json:"asset"`
	Tag          string `json:"tag"`
	AssetName    string `json:"asset_name"`
	DownloadedAt string `json:"downloaded_at"`
}
