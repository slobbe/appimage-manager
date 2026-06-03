package domain

import (
	"strings"
)

const (
	ProviderGitHub = "github"
)

type PackageRef struct {
	Kind        string `json:"kind"`
	ProviderRef string `json:"provider_ref"`
}

type PackageMetadata struct {
	Name            string
	Provider        string
	Ref             PackageRef
	RepoURL         string
	LatestVersion   string
	AssetName       string
	AssetPattern    string
	DownloadURL     string
	AssetCandidates []AssetCandidate
	AssetAmbiguous  bool
	AssetReason     string
	Installable     bool
	InstallReason   string
	ReleaseTag      string
	Summary         string
}

type AssetCandidate struct {
	Name        string
	DownloadURL string
	Arch        string
	ArchLabel   string
}

func FormatPackageRef(ref PackageRef) string {
	kind := strings.TrimSpace(ref.Kind)
	value := strings.TrimSpace(ref.ProviderRef)
	if kind == "" || value == "" {
		return ""
	}
	return kind + ":" + value
}
