package github

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

type rewriteHostTransport struct {
	base *url.URL
	next http.RoundTripper
}

func (t *rewriteHostTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	clone := req.Clone(req.Context())
	clone.URL.Scheme = t.base.Scheme
	clone.URL.Host = t.base.Host
	clone.Host = t.base.Host
	return t.next.RoundTrip(clone)
}

func TestFetchRepository(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/owner/repo" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if got := r.Header.Get("Accept"); got != "application/vnd.github+json" {
			t.Fatalf("Accept = %q, want github json header", got)
		}
		_, _ = w.Write([]byte(`{"name":"repo","full_name":"owner/repo","description":"Example","html_url":"https://github.com/owner/repo","stargazers_count":42}`))
	}))
	defer server.Close()

	originalClient := SetRepositoryHTTPClient(nil)
	t.Cleanup(func() {
		SetRepositoryHTTPClient(originalClient)
	})

	baseURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("failed to parse server URL: %v", err)
	}
	SetRepositoryHTTPClient(&http.Client{
		Transport: &rewriteHostTransport{
			base: baseURL,
			next: server.Client().Transport,
		},
	})

	repository, err := FetchRepository(context.Background(), "owner/repo")
	if err != nil {
		t.Fatalf("FetchRepository returned error: %v", err)
	}
	if repository.Name != "repo" {
		t.Fatalf("Name = %q, want repo", repository.Name)
	}
	if repository.HTMLURL != "https://github.com/owner/repo" {
		t.Fatalf("HTMLURL = %q", repository.HTMLURL)
	}
	if repository.StargazersCount != 42 {
		t.Fatalf("StargazersCount = %d, want 42", repository.StargazersCount)
	}
}
