package discovery

import (
	"fmt"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/slobbe/appimage-manager/internal/domain"
)

func ParseGitHubRepoValue(value string) (domain.PackageRef, error) {
	providerRef := strings.TrimSpace(value)
	if providerRef == "" || strings.Count(providerRef, "/") != 1 || strings.HasPrefix(providerRef, "/") || strings.HasSuffix(providerRef, "/") {
		return domain.PackageRef{}, fmt.Errorf("github repo must be in the form owner/repo")
	}
	return domain.PackageRef{Kind: domain.ProviderGitHub, ProviderRef: providerRef}, nil
}

func ParsePackageRefURL(input string) (domain.PackageRef, error) {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return domain.PackageRef{}, fmt.Errorf("unsupported package ref URL %q", input)
	}

	u, err := url.Parse(trimmed)
	if err != nil {
		return domain.PackageRef{}, fmt.Errorf("unsupported package ref URL %q", input)
	}
	if !strings.EqualFold(u.Scheme, "https") {
		return domain.PackageRef{}, fmt.Errorf("unsupported package ref URL %q", input)
	}

	switch normalizeProviderHost(u.Hostname()) {
	case "github.com":
		if ref, ok := parseGitHubPackageRefURL(u); ok {
			return ref, nil
		}
	}

	return domain.PackageRef{}, fmt.Errorf("unsupported package ref URL %q", input)
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

func normalizeProviderHost(host string) string {
	normalized := strings.ToLower(strings.TrimSpace(host))
	return strings.TrimPrefix(normalized, "www.")
}

func parseGitHubPackageRefURL(u *url.URL) (domain.PackageRef, bool) {
	segments := splitPathSegments(u.Path)
	if len(segments) < 2 {
		return domain.PackageRef{}, false
	}

	ref, err := ParseGitHubRepoValue(segments[0] + "/" + segments[1])
	if err != nil {
		return domain.PackageRef{}, false
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

	return domain.PackageRef{}, false
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
