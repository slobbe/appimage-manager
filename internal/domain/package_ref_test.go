package domain

import "testing"

func TestParseGitHubRepoValue(t *testing.T) {
	ref, err := ParseGitHubRepoValue("owner/repo")
	if err != nil {
		t.Fatalf("ParseGitHubRepoValue returned error: %v", err)
	}
	if ref.Kind != ProviderGitHub || ref.ProviderRef != "owner/repo" {
		t.Fatalf("ParseGitHubRepoValue returned %+v", ref)
	}

	if _, err := ParseGitHubRepoValue("owner/repo/extra"); err == nil {
		t.Fatal("ParseGitHubRepoValue accepted malformed ref")
	}
}

func TestParsePackageRefURLAcceptsGitHubProjectURLs(t *testing.T) {
	tests := []string{
		"https://github.com/owner/repo",
		"https://www.github.com/owner/repo/releases/tag/v1.0.0",
		"https://github.com/owner/repo/tree/main",
		"https://github.com/owner/repo/blob/main/App.AppImage",
	}

	for _, input := range tests {
		t.Run(input, func(t *testing.T) {
			ref, err := ParsePackageRefURL(input)
			if err != nil {
				t.Fatalf("ParsePackageRefURL returned error: %v", err)
			}
			if ref.Kind != ProviderGitHub || ref.ProviderRef != "owner/repo" {
				t.Fatalf("ParsePackageRefURL returned %+v", ref)
			}
		})
	}
}

func TestParsePackageRefURLRejectsUnsupportedURLs(t *testing.T) {
	for _, input := range []string{"http://github.com/owner/repo", "https://example.com/owner/repo", "https://github.com/owner"} {
		t.Run(input, func(t *testing.T) {
			if _, err := ParsePackageRefURL(input); err == nil {
				t.Fatal("ParsePackageRefURL accepted unsupported URL")
			}
		})
	}
}

func TestDisplayNameFromRef(t *testing.T) {
	if got := DisplayNameFromRef("owner/my-app"); got != "my app" {
		t.Fatalf("DisplayNameFromRef = %q, want %q", got, "my app")
	}
}
