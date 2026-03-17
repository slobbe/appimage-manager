package discovery

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
)

const (
	ProviderGitHub = "github"
	ProviderGitLab = "gitlab"
)

type PackageRef struct {
	Kind        string `json:"kind"`
	ProviderRef string `json:"provider_ref"`
}

type PackageMetadata struct {
	Name          string
	Provider      string
	Ref           PackageRef
	RepoURL       string
	LatestVersion string
	AssetName     string
	AssetPattern  string
	DownloadURL   string
	Installable   bool
	InstallReason string
	ReleaseTag    string
	Summary       string
}

type DiscoveryBackend interface {
	Name() string
	Resolve(ctx context.Context, ref PackageRef, assetOverride string) (*PackageMetadata, error)
}

func ParsePackageRef(input string) (PackageRef, error) {
	trimmed := strings.TrimSpace(input)
	switch {
	case strings.HasPrefix(trimmed, "github:"):
		providerRef := strings.TrimSpace(strings.TrimPrefix(trimmed, "github:"))
		if providerRef == "" || strings.Count(providerRef, "/") != 1 {
			return PackageRef{}, fmt.Errorf("github package ref must be in the form github:owner/repo")
		}
		return PackageRef{Kind: ProviderGitHub, ProviderRef: providerRef}, nil
	case strings.HasPrefix(trimmed, "gitlab:"):
		providerRef := strings.TrimSpace(strings.TrimPrefix(trimmed, "gitlab:"))
		if providerRef == "" || strings.Count(providerRef, "/") < 1 || strings.HasPrefix(providerRef, "/") || strings.HasSuffix(providerRef, "/") {
			return PackageRef{}, fmt.Errorf("gitlab package ref must be in the form gitlab:namespace/project")
		}
		return PackageRef{Kind: ProviderGitLab, ProviderRef: providerRef}, nil
	default:
		return PackageRef{}, fmt.Errorf("unsupported package ref %q", input)
	}
}

func FormatPackageRef(ref PackageRef) string {
	kind := strings.TrimSpace(ref.Kind)
	value := strings.TrimSpace(ref.ProviderRef)
	if kind == "" || value == "" {
		return ""
	}
	return kind + ":" + value
}

func DisplayNameFromRef(value string) string {
	base := filepath.Base(strings.TrimSpace(value))
	if base == "" || base == "." || base == string(filepath.Separator) {
		return strings.TrimSpace(value)
	}

	base = strings.TrimSpace(base)
	if base == "" {
		return strings.TrimSpace(value)
	}

	return strings.ReplaceAll(base, "-", " ")
}
