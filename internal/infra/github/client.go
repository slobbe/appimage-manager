package github

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/slobbe/appimage-manager/internal/app"
)

const (
	defaultBaseURL     = "https://api.github.com"
	maxRequestAttempts = 3
	retryBaseDelay     = 100 * time.Millisecond
)

var defaultHTTPClient = &http.Client{Timeout: 30 * time.Second}

// Client looks up release metadata from the GitHub REST API.
type Client struct {
	HTTPClient *http.Client
	BaseURL    string
}

// NewClient creates a GitHub release finder that uses the public GitHub API.
func NewClient(httpClient *http.Client) Client {
	return Client{HTTPClient: httpClient, BaseURL: defaultBaseURL}
}

var _ app.GitHubReleaseFinder = Client{}

func (c Client) LatestRelease(ctx context.Context, repo string, includePrerelease bool) (app.GitHubRelease, error) {
	owner, name, normalizedRepo, err := parseRepo(repo)
	if err != nil {
		return app.GitHubRelease{}, err
	}

	requestURL, err := c.releaseURL(owner, name, includePrerelease)
	if err != nil {
		return app.GitHubRelease{}, err
	}

	return c.fetchRelease(ctx, normalizedRepo, requestURL, includePrerelease, func(releases []githubReleaseResponse) (githubReleaseResponse, error) {
		for _, release := range releases {
			if !release.Draft {
				return release, nil
			}
		}
		return githubReleaseResponse{}, fmt.Errorf("find github release for %s: no non-draft releases found", normalizedRepo)
	})
}

func (c Client) LatestPrerelease(ctx context.Context, repo string) (app.GitHubRelease, error) {
	owner, name, normalizedRepo, err := parseRepo(repo)
	if err != nil {
		return app.GitHubRelease{}, err
	}

	requestURL, err := c.releasesURL(owner, name)
	if err != nil {
		return app.GitHubRelease{}, err
	}

	return c.fetchRelease(ctx, normalizedRepo, requestURL, true, func(releases []githubReleaseResponse) (githubReleaseResponse, error) {
		for _, release := range releases {
			if !release.Draft && release.Prerelease {
				return release, nil
			}
		}
		return githubReleaseResponse{}, fmt.Errorf("find github prerelease for %s: no non-draft prereleases found", normalizedRepo)
	})
}

func (c Client) ReleaseByTag(ctx context.Context, repo string, tag string) (app.GitHubRelease, error) {
	owner, name, normalizedRepo, err := parseRepo(repo)
	if err != nil {
		return app.GitHubRelease{}, err
	}
	tag = strings.TrimSpace(tag)
	if tag == "" {
		return app.GitHubRelease{}, errors.New("github release tag is required")
	}

	requestURL, err := c.releaseByTagURL(owner, name, tag)
	if err != nil {
		return app.GitHubRelease{}, err
	}

	return c.fetchSingleRelease(ctx, normalizedRepo, requestURL, "github release "+tag)
}

func (c Client) fetchRelease(ctx context.Context, repo string, requestURL string, list bool, selectRelease func([]githubReleaseResponse) (githubReleaseResponse, error)) (app.GitHubRelease, error) {
	if err := ctx.Err(); err != nil {
		return app.GitHubRelease{}, err
	}

	if !list {
		return c.fetchSingleRelease(ctx, repo, requestURL, "latest github release")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return app.GitHubRelease{}, fmt.Errorf("create github release request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("User-Agent", "aim")

	resp, err := c.do(req)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return app.GitHubRelease{}, ctxErr
		}
		return app.GitHubRelease{}, fmt.Errorf("fetch github releases for %s: %w", repo, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return app.GitHubRelease{}, fmt.Errorf("fetch github releases for %s: %s", repo, githubHTTPError(resp))
	}

	var releases []githubReleaseResponse
	if err := json.NewDecoder(resp.Body).Decode(&releases); err != nil {
		return app.GitHubRelease{}, fmt.Errorf("decode github releases for %s: %w", repo, err)
	}
	release, err := selectRelease(releases)
	if err != nil {
		return app.GitHubRelease{}, err
	}

	return release.toAppRelease(repo), nil
}

func (c Client) fetchSingleRelease(ctx context.Context, repo string, requestURL string, label string) (app.GitHubRelease, error) {
	if err := ctx.Err(); err != nil {
		return app.GitHubRelease{}, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return app.GitHubRelease{}, fmt.Errorf("create github release request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("User-Agent", "aim")

	resp, err := c.do(req)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return app.GitHubRelease{}, ctxErr
		}
		return app.GitHubRelease{}, fmt.Errorf("fetch %s for %s: %w", label, repo, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return app.GitHubRelease{}, fmt.Errorf("fetch %s for %s: %s", label, repo, githubHTTPError(resp))
	}

	var release githubReleaseResponse
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return app.GitHubRelease{}, fmt.Errorf("decode %s for %s: %w", label, repo, err)
	}

	return release.toAppRelease(repo), nil
}

func (c Client) releaseURL(owner string, repo string, includePrerelease bool) (string, error) {
	if includePrerelease {
		return c.releasesURL(owner, repo)
	}
	return c.githubURL("/repos/" + url.PathEscape(owner) + "/" + url.PathEscape(repo) + "/releases/latest")
}

func (c Client) releasesURL(owner string, repo string) (string, error) {
	return c.githubURL("/repos/" + url.PathEscape(owner) + "/" + url.PathEscape(repo) + "/releases")
}

func (c Client) releaseByTagURL(owner string, repo string, tag string) (string, error) {
	return c.githubURL("/repos/" + url.PathEscape(owner) + "/" + url.PathEscape(repo) + "/releases/tags/" + url.PathEscape(tag))
}

func (c Client) githubURL(path string) (string, error) {
	base := strings.TrimSpace(c.BaseURL)
	if base == "" {
		base = defaultBaseURL
	}
	parsed, err := url.Parse(base)
	if err != nil {
		return "", fmt.Errorf("parse github base url %q: %w", base, err)
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/") + path
	parsed.RawQuery = ""
	parsed.Fragment = ""

	return parsed.String(), nil
}

func parseRepo(repo string) (string, string, string, error) {
	repo = strings.TrimSpace(repo)
	owner, name, ok := strings.Cut(repo, "/")
	if !ok || owner == "" || name == "" || strings.Contains(name, "/") {
		return "", "", "", errors.New("github repo must be in owner/repo format")
	}
	return owner, name, owner + "/" + name, nil
}

func (c Client) httpClient() *http.Client {
	if c.HTTPClient != nil {
		return c.HTTPClient
	}

	return defaultHTTPClient
}

func (c Client) do(req *http.Request) (*http.Response, error) {
	var lastErr error
	for attempt := 0; attempt < maxRequestAttempts; attempt++ {
		if attempt > 0 {
			timer := time.NewTimer(retryBaseDelay * time.Duration(1<<(attempt-1)))
			select {
			case <-req.Context().Done():
				timer.Stop()
				return nil, req.Context().Err()
			case <-timer.C:
			}
		}

		resp, err := c.httpClient().Do(req.Clone(req.Context()))
		if err == nil {
			if !isTransientStatus(resp.StatusCode) || attempt == maxRequestAttempts-1 {
				return resp, nil
			}
			resp.Body.Close()
			continue
		}
		if resp != nil {
			resp.Body.Close()
		}
		if err := req.Context().Err(); err != nil {
			return nil, err
		}
		lastErr = err
	}
	return nil, lastErr
}

func isTransientStatus(status int) bool {
	return status == http.StatusBadGateway ||
		status == http.StatusServiceUnavailable ||
		status == http.StatusGatewayTimeout
}

func githubHTTPError(resp *http.Response) string {
	message := "github returned " + resp.Status
	if resp.StatusCode != http.StatusForbidden && resp.StatusCode != http.StatusTooManyRequests {
		return message
	}

	if retryAfter := strings.TrimSpace(resp.Header.Get("Retry-After")); retryAfter != "" {
		if seconds, err := strconv.ParseInt(retryAfter, 10, 64); err == nil {
			return fmt.Sprintf("%s; rate limited, retry after %d seconds", message, seconds)
		}
		if retryAt, err := http.ParseTime(retryAfter); err == nil {
			return fmt.Sprintf("%s; rate limited, retry after %s", message, retryAt.UTC().Format(time.RFC3339))
		}
	}
	if reset := strings.TrimSpace(resp.Header.Get("X-RateLimit-Reset")); reset != "" {
		if unix, err := strconv.ParseInt(reset, 10, 64); err == nil {
			return fmt.Sprintf("%s; rate limited, retry after %s", message, time.Unix(unix, 0).UTC().Format(time.RFC3339))
		}
	}
	if remaining := strings.TrimSpace(resp.Header.Get("X-RateLimit-Remaining")); remaining == "0" {
		return message + "; GitHub API rate limit exhausted"
	}
	if resp.StatusCode == http.StatusForbidden {
		return message
	}
	return message + "; GitHub API request was rate limited"
}

type githubReleaseResponse struct {
	TagName    string                       `json:"tag_name"`
	Name       string                       `json:"name"`
	HTMLURL    string                       `json:"html_url"`
	Prerelease bool                         `json:"prerelease"`
	Draft      bool                         `json:"draft"`
	Assets     []githubReleaseAssetResponse `json:"assets"`
}

type githubReleaseAssetResponse struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
	ContentType        string `json:"content_type"`
	Size               int64  `json:"size"`
}

func (r githubReleaseResponse) toAppRelease(repo string) app.GitHubRelease {
	assets := make([]app.GitHubReleaseAsset, 0, len(r.Assets))
	for _, asset := range r.Assets {
		assets = append(assets, app.GitHubReleaseAsset{
			Name:        asset.Name,
			DownloadURL: asset.BrowserDownloadURL,
			ContentType: asset.ContentType,
			SizeBytes:   asset.Size,
		})
	}

	return app.GitHubRelease{
		Repo:       repo,
		TagName:    r.TagName,
		Name:       r.Name,
		URL:        r.HTMLURL,
		Prerelease: r.Prerelease,
		Draft:      r.Draft,
		Assets:     assets,
	}
}
