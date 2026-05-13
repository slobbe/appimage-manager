package github

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/slobbe/appimage-manager/internal/infra/httpclient"
)

type Repository struct {
	Name            string
	FullName        string
	Description     string
	HTMLURL         string
	StargazersCount int
}

type repositoryResponse struct {
	Name            string `json:"name"`
	FullName        string `json:"full_name"`
	Description     string `json:"description"`
	HTMLURL         string `json:"html_url"`
	StargazersCount int    `json:"stargazers_count"`
}

var repositoryHTTPClient = httpclient.New(30 * time.Second)

func SetRepositoryHTTPClientTimeout(timeout time.Duration) {
	if repositoryHTTPClient == nil {
		repositoryHTTPClient = httpclient.New(timeout)
		return
	}
	repositoryHTTPClient.Timeout = timeout
}

func SetRepositoryHTTPClient(client *http.Client) *http.Client {
	previous := repositoryHTTPClient
	repositoryHTTPClient = client
	return previous
}

func FetchRepository(ctx context.Context, repoSlug string) (*Repository, error) {
	requestURL := fmt.Sprintf("https://api.github.com/repos/%s", repoSlug)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := repositoryHTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, fmt.Errorf("github repo api returned status %s", resp.Status)
	}

	var payload repositoryResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}

	return &Repository{
		Name:            payload.Name,
		FullName:        payload.FullName,
		Description:     payload.Description,
		HTMLURL:         payload.HTMLURL,
		StargazersCount: payload.StargazersCount,
	}, nil
}
