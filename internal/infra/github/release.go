package github

import (
	"encoding/json"
	"fmt"
	"net/http"
	"path"
	"runtime"
	"strings"
	"time"

	models "github.com/slobbe/appimage-manager/internal/domain"
	"github.com/slobbe/appimage-manager/internal/infra/httpclient"
)

type ReleaseAsset struct {
	DownloadURL       string
	TagName           string
	NormalizedVersion string
	AssetName         string
	PreRelease        bool
}

type ReleaseAssetCandidate struct {
	Name        string
	DownloadURL string
	Arch        string
	ArchLabel   string
}

type ReleaseAssetSelection struct {
	Release    *ReleaseAsset
	Candidates []ReleaseAssetCandidate
	Ambiguous  bool
	Reason     string
}

type releaseResponse struct {
	TagName    string     `json:"tag_name"`
	Prerelease bool       `json:"prerelease"`
	Draft      bool       `json:"draft"`
	Assets     []apiAsset `json:"assets"`
}

type apiAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

type assetMatch struct {
	name  string
	url   string
	arch  string
	label string
}

type assetSelection struct {
	selected   assetMatch
	candidates []assetMatch
	ambiguous  bool
	reason     string
}

var releaseHTTPClient = httpclient.New(30 * time.Second)

func SetReleaseHTTPClient(client *http.Client) *http.Client {
	previous := releaseHTTPClient
	releaseHTTPClient = client
	return previous
}

func ResolveReleaseAsset(repoSlug, assetPattern string) (*ReleaseAsset, error) {
	selection, err := ResolveReleaseAssetSelection(repoSlug, assetPattern, runtime.GOARCH)
	if err != nil {
		return nil, err
	}
	if selection.Ambiguous {
		return nil, fmt.Errorf("%s: %s", selection.Reason, FormatAssetCandidateNames(selection.Candidates))
	}
	return selection.Release, nil
}

func ResolveReleaseAssetSelection(repoSlug, assetPattern, arch string) (*ReleaseAssetSelection, error) {
	repoSlug = strings.TrimSpace(repoSlug)
	if repoSlug == "" || strings.Count(repoSlug, "/") != 1 {
		return nil, fmt.Errorf("invalid github repo %q", repoSlug)
	}

	assetPattern = strings.TrimSpace(assetPattern)
	if assetPattern == "" {
		return nil, fmt.Errorf("missing github asset pattern")
	}

	payload, err := fetchReleases(repoSlug)
	if err != nil {
		return nil, err
	}

	release, ok := selectRelease(payload, false)
	if !ok {
		return nil, fmt.Errorf("no matching github releases found")
	}

	if strings.TrimSpace(arch) == "" {
		arch = runtime.GOARCH
	}
	assetSelection := matchAssetSelection(release.Assets, assetPattern, arch)
	if len(assetSelection.candidates) == 0 {
		return nil, fmt.Errorf("no assets match pattern %q", assetPattern)
	}
	candidates := releaseAssetCandidates(assetSelection.candidates)

	if assetSelection.ambiguous {
		return &ReleaseAssetSelection{
			Candidates: candidates,
			Ambiguous:  true,
			Reason:     assetSelection.reason,
		}, nil
	}

	selected := assetSelection.selected
	return &ReleaseAssetSelection{
		Release: &ReleaseAsset{
			DownloadURL:       selected.url,
			TagName:           release.TagName,
			NormalizedVersion: models.NormalizeComparableVersion(release.TagName),
			AssetName:         selected.name,
			PreRelease:        release.Prerelease,
		},
		Candidates: candidates,
	}, nil
}

func fetchReleases(repoSlug string) ([]releaseResponse, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/releases", repoSlug)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := releaseHTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, fmt.Errorf("github api returned status %s", resp.Status)
	}

	var payload []releaseResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}

	return payload, nil
}

func selectRelease(releases []releaseResponse, allowPrerelease bool) (releaseResponse, bool) {
	for _, release := range releases {
		if release.Draft {
			continue
		}
		if !allowPrerelease && release.Prerelease {
			continue
		}
		return release, true
	}
	return releaseResponse{}, false
}

func matchAsset(assets []apiAsset, pattern, arch string) (string, string) {
	selection := matchAssetSelection(assets, pattern, arch)
	if selection.ambiguous || selection.selected.url == "" {
		return "", ""
	}
	return selection.selected.name, selection.selected.url
}

func matchAssetSelection(assets []apiAsset, pattern, arch string) assetSelection {
	var matches []assetMatch
	for _, asset := range assets {
		ok, err := path.Match(pattern, asset.Name)
		if err == nil && ok {
			assetArch, label := classifyAssetArch(asset.Name)
			matches = append(matches, assetMatch{name: asset.Name, url: asset.BrowserDownloadURL, arch: assetArch, label: label})
		}
	}

	if len(matches) == 0 {
		return assetSelection{}
	}

	return selectBestAsset(matches, arch)
}

func selectBestAsset(matches []assetMatch, arch string) assetSelection {
	arch = normalizeGOArch(arch)

	var archHits, generic []assetMatch

	for _, match := range matches {
		switch {
		case match.arch == arch && arch != "":
			archHits = append(archHits, match)
		case match.arch == "":
			generic = append(generic, match)
		}
	}

	if len(archHits) == 1 {
		return assetSelection{selected: archHits[0], candidates: matches}
	}
	if len(archHits) > 1 {
		return assetSelection{candidates: archHits, ambiguous: true, reason: "multiple assets match local architecture " + arch}
	}
	if len(generic) == 1 {
		return assetSelection{selected: generic[0], candidates: matches}
	}
	if len(generic) > 1 {
		return assetSelection{candidates: generic, ambiguous: true, reason: "multiple generic assets match"}
	}
	return assetSelection{candidates: matches, ambiguous: true, reason: "no asset matches local architecture " + arch}
}

func normalizeGOArch(arch string) string {
	arch = strings.ToLower(strings.TrimSpace(arch))
	switch arch {
	case "amd64":
		return "amd64"
	case "386":
		return "386"
	case "arm64":
		return "arm64"
	case "arm":
		return "arm"
	case "riscv64", "ppc64le", "s390x":
		return arch
	default:
		return arch
	}
}

func classifyAssetArch(name string) (string, string) {
	normalized := strings.ToLower(strings.TrimSpace(name))
	if normalized == "" {
		return "", "generic"
	}

	phraseChecks := []struct {
		token string
		arch  string
		label string
	}{
		{token: "x86-64", arch: "amd64", label: "x86-64"},
		{token: "x86_64", arch: "amd64", label: "x86_64"},
	}
	for _, check := range phraseChecks {
		if containsDelimitedToken(normalized, check.token) {
			return check.arch, check.label
		}
	}

	tokens := splitAssetNameTokens(normalized)
	for _, token := range tokens {
		if arch, ok := knownArchToken(token); ok {
			return arch, token
		}
	}
	return "", "generic"
}

func knownArchToken(token string) (string, bool) {
	switch token {
	case "amd64", "x86_64", "x64":
		return "amd64", true
	case "386", "i386", "i686", "x86":
		return "386", true
	case "arm64", "aarch64", "armv8":
		return "arm64", true
	case "arm", "armv6", "armv6l", "armv7", "armv7l", "armhf", "arm32":
		return "arm", true
	case "riscv64":
		return "riscv64", true
	case "ppc64le":
		return "ppc64le", true
	case "s390x":
		return "s390x", true
	default:
		return "", false
	}
}

func splitAssetNameTokens(name string) []string {
	return strings.FieldsFunc(name, func(r rune) bool {
		switch r {
		case '.', '_', '-', ' ', '\t', '\n', '\r', '/', '\\', '(', ')', '[', ']':
			return true
		default:
			return false
		}
	})
}

func containsDelimitedToken(name, token string) bool {
	index := strings.Index(name, token)
	for index >= 0 {
		before := index == 0 || isAssetDelimiter(rune(name[index-1]))
		afterIndex := index + len(token)
		after := afterIndex == len(name) || isAssetDelimiter(rune(name[afterIndex]))
		if before && after {
			return true
		}
		next := strings.Index(name[index+1:], token)
		if next < 0 {
			return false
		}
		index += next + 1
	}
	return false
}

func isAssetDelimiter(r rune) bool {
	switch r {
	case '.', '_', '-', ' ', '\t', '\n', '\r', '/', '\\', '(', ')', '[', ']':
		return true
	default:
		return false
	}
}

func releaseAssetCandidates(matches []assetMatch) []ReleaseAssetCandidate {
	candidates := make([]ReleaseAssetCandidate, 0, len(matches))
	for _, match := range matches {
		candidates = append(candidates, ReleaseAssetCandidate{
			Name:        match.name,
			DownloadURL: match.url,
			Arch:        match.arch,
			ArchLabel:   match.label,
		})
	}
	return candidates
}

func FormatAssetCandidateNames(candidates []ReleaseAssetCandidate) string {
	names := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		if strings.TrimSpace(candidate.Name) != "" {
			names = append(names, candidate.Name)
		}
	}
	if len(names) == 0 {
		return "no candidate assets"
	}
	return "candidates: " + strings.Join(names, ", ")
}
