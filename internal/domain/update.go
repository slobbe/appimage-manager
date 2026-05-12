package domain

type UpdateKind string

const (
	UpdateNone          UpdateKind = "none"
	UpdateZsync         UpdateKind = "zsync"
	UpdateGitHubRelease UpdateKind = "github_release"
)

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
