package github

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/slobbe/appimage-manager/internal/app"
)

// LatestReleaseMatchingTag returns the first eligible release in GitHub list order
// whose tag matches tagPattern using filepath.Match.
func (c Client) LatestReleaseMatchingTag(ctx context.Context, repo string, includePrerelease bool, tagPattern string) (app.GitHubRelease, error) {
	owner, name, normalizedRepo, err := parseRepo(repo)
	if err != nil {
		return app.GitHubRelease{}, err
	}
	if err := validateTagPattern(tagPattern); err != nil {
		return app.GitHubRelease{}, fmt.Errorf("invalid github release tag pattern %q: %w", tagPattern, err)
	}
	requestURL, err := c.releasesURL(owner, name)
	if err != nil {
		return app.GitHubRelease{}, err
	}

	for requestURL != "" {
		releases, nextURL, err := c.fetchMatchingTagPage(ctx, normalizedRepo, requestURL)
		if err != nil {
			return app.GitHubRelease{}, err
		}
		for _, release := range releases {
			if release.Draft || (release.Prerelease && !includePrerelease) {
				continue
			}
			matched, err := filepath.Match(tagPattern, release.TagName)
			if err != nil {
				return app.GitHubRelease{}, fmt.Errorf("match github release tag pattern %q: %w", tagPattern, err)
			}
			if matched {
				return release.toAppRelease(normalizedRepo), nil
			}
		}
		requestURL = nextURL
	}
	return app.GitHubRelease{}, fmt.Errorf("find github release for %s: no eligible releases matching tag pattern %q", normalizedRepo, tagPattern)
}

// Match can stop after a nonmatching chunk without validating later chunks.
// Validate each star-separated chunk separately, preserving escapes and classes.
func validateTagPattern(pattern string) error {
	start := 0
	inClass := false
	for i := 0; i < len(pattern); i++ {
		switch pattern[i] {
		case '\\':
			if runtime.GOOS != "windows" && i+1 < len(pattern) {
				i++
			}
		case '[':
			inClass = true
		case ']':
			inClass = false
		case '*':
			if !inClass {
				if _, err := filepath.Match(pattern[start:i], ""); err != nil {
					return err
				}
				start = i + 1
			}
		}
	}
	_, err := filepath.Match(pattern[start:], "")
	return err
}

func (c Client) fetchMatchingTagPage(ctx context.Context, repo string, requestURL string) ([]githubReleaseResponse, string, error) {
	if err := ctx.Err(); err != nil {
		return nil, "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return nil, "", fmt.Errorf("create github release request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("User-Agent", "aim")
	resp, err := c.do(req)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, "", ctxErr
		}
		return nil, "", fmt.Errorf("fetch github releases for %s: %w", repo, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, "", fmt.Errorf("fetch github releases for %s: %s", repo, githubHTTPError(resp))
	}
	var releases []githubReleaseResponse
	if err := json.NewDecoder(resp.Body).Decode(&releases); err != nil {
		return nil, "", fmt.Errorf("decode github releases for %s: %w", repo, err)
	}
	for _, header := range resp.Header.Values("Link") {
		for _, link := range strings.Split(header, ",") {
			parts := strings.Split(link, ";")
			for _, param := range parts[1:] {
				key, value, ok := strings.Cut(strings.TrimSpace(param), "=")
				if !ok || key != "rel" {
					continue
				}
				for _, relation := range strings.Fields(strings.Trim(value, "\"")) {
					if relation == "next" {
						next, err := req.URL.Parse(strings.Trim(strings.TrimSpace(parts[0]), "<>"))
						if err != nil {
							return nil, "", fmt.Errorf("parse github releases next page: %w", err)
						}
						return releases, next.String(), nil
					}
				}
			}
		}
	}
	return releases, "", nil
}
