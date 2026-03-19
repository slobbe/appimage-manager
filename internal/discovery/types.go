package discovery

import (
	"context"
	"fmt"
	"net/url"
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

func ParseGitHubRepoValue(value string) (PackageRef, error) {
	providerRef := strings.TrimSpace(value)
	if providerRef == "" || strings.Count(providerRef, "/") != 1 || strings.HasPrefix(providerRef, "/") || strings.HasSuffix(providerRef, "/") {
		return PackageRef{}, fmt.Errorf("github repo must be in the form owner/repo")
	}
	return PackageRef{Kind: ProviderGitHub, ProviderRef: providerRef}, nil
}

func ParseGitLabProjectValue(value string) (PackageRef, error) {
	providerRef := strings.TrimSpace(value)
	if providerRef == "" || strings.Count(providerRef, "/") < 1 || strings.HasPrefix(providerRef, "/") || strings.HasSuffix(providerRef, "/") {
		return PackageRef{}, fmt.Errorf("gitlab project must be in the form namespace/project")
	}
	return PackageRef{Kind: ProviderGitLab, ProviderRef: providerRef}, nil
}

func ParsePackageRefURL(input string) (PackageRef, error) {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return PackageRef{}, fmt.Errorf("unsupported package ref URL %q", input)
	}

	u, err := url.Parse(trimmed)
	if err != nil {
		return PackageRef{}, fmt.Errorf("unsupported package ref URL %q", input)
	}
	if !strings.EqualFold(u.Scheme, "https") {
		return PackageRef{}, fmt.Errorf("unsupported package ref URL %q", input)
	}

	switch normalizeProviderHost(u.Hostname()) {
	case "github.com":
		if ref, ok := parseGitHubPackageRefURL(u); ok {
			return ref, nil
		}
	case "gitlab.com":
		if ref, ok := parseGitLabPackageRefURL(u); ok {
			return ref, nil
		}
	}

	return PackageRef{}, fmt.Errorf("unsupported package ref URL %q", input)
}

func normalizeProviderHost(host string) string {
	normalized := strings.ToLower(strings.TrimSpace(host))
	return strings.TrimPrefix(normalized, "www.")
}

func parseGitHubPackageRefURL(u *url.URL) (PackageRef, bool) {
	segments := splitPathSegments(u.Path)
	if len(segments) < 2 {
		return PackageRef{}, false
	}

	ref, err := ParseGitHubRepoValue(segments[0] + "/" + segments[1])
	if err != nil {
		return PackageRef{}, false
	}

	if len(segments) == 2 {
		return ref, true
	}

	switch segments[2] {
	case "releases":
		if len(segments) == 3 {
			return ref, true
		}
		if len(segments) >= 5 && segments[3] == "tag" {
			return ref, true
		}
	case "tags":
		if len(segments) == 3 {
			return ref, true
		}
	case "tree":
		if len(segments) >= 4 {
			return ref, true
		}
	case "blob":
		if len(segments) >= 5 {
			return ref, true
		}
	}

	return PackageRef{}, false
}

func parseGitLabPackageRefURL(u *url.URL) (PackageRef, bool) {
	segments := splitPathSegments(u.Path)
	if len(segments) < 2 {
		return PackageRef{}, false
	}

	dashIndex := -1
	for idx, segment := range segments {
		if segment == "-" {
			dashIndex = idx
			break
		}
	}

	if dashIndex == -1 {
		ref, err := ParseGitLabProjectValue(strings.Join(segments, "/"))
		if err != nil {
			return PackageRef{}, false
		}
		return ref, true
	}

	projectSegments := segments[:dashIndex]
	if len(projectSegments) < 2 {
		return PackageRef{}, false
	}

	ref, err := ParseGitLabProjectValue(strings.Join(projectSegments, "/"))
	if err != nil {
		return PackageRef{}, false
	}

	if len(segments) <= dashIndex+1 {
		return PackageRef{}, false
	}

	switch segments[dashIndex+1] {
	case "releases":
		if len(segments) >= dashIndex+2 {
			return ref, true
		}
	case "tags":
		if len(segments) == dashIndex+2 {
			return ref, true
		}
	case "tree":
		if len(segments) >= dashIndex+3 {
			return ref, true
		}
	case "blob":
		if len(segments) >= dashIndex+4 {
			return ref, true
		}
	}

	return PackageRef{}, false
}

func splitPathSegments(path string) []string {
	rawSegments := strings.Split(strings.TrimSpace(path), "/")
	segments := make([]string, 0, len(rawSegments))
	for _, segment := range rawSegments {
		trimmed := strings.TrimSpace(segment)
		if trimmed == "" {
			continue
		}
		decoded, err := url.PathUnescape(trimmed)
		if err == nil {
			trimmed = decoded
		}
		segments = append(segments, trimmed)
	}
	return segments
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
