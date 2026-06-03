package discovery

import (
	"testing"

	"github.com/slobbe/appimage-manager/internal/domain"
)

func TestParseGitHubRepoValue(t *testing.T) {
	tests := []struct {
		input     string
		expect    domain.PackageRef
		wantError bool
	}{
		{input: "owner/repo", expect: domain.PackageRef{Kind: domain.ProviderGitHub, ProviderRef: "owner/repo"}},
		{input: "owner", wantError: true},
		{input: "/owner/repo", wantError: true},
	}

	for _, tt := range tests {
		got, err := ParseGitHubRepoValue(tt.input)
		if tt.wantError {
			if err == nil {
				t.Fatalf("ParseGitHubRepoValue(%q) expected error", tt.input)
			}
			continue
		}
		if err != nil {
			t.Fatalf("ParseGitHubRepoValue(%q) returned error: %v", tt.input, err)
		}
		if got != tt.expect {
			t.Fatalf("ParseGitHubRepoValue(%q) = %#v, want %#v", tt.input, got, tt.expect)
		}
	}
}

func TestParsePackageRefURL(t *testing.T) {
	tests := []struct {
		input     string
		expect    domain.PackageRef
		wantError bool
	}{
		{input: "https://github.com/owner/repo", expect: domain.PackageRef{Kind: domain.ProviderGitHub, ProviderRef: "owner/repo"}},
		{input: "https://www.github.com/owner/repo/releases", expect: domain.PackageRef{Kind: domain.ProviderGitHub, ProviderRef: "owner/repo"}},
		{input: "https://github.com/owner/repo/releases/tag/v1.2.3?tab=readme#fragment", expect: domain.PackageRef{Kind: domain.ProviderGitHub, ProviderRef: "owner/repo"}},
		{input: "https://github.com/owner/repo/blob/main/README.md", expect: domain.PackageRef{Kind: domain.ProviderGitHub, ProviderRef: "owner/repo"}},
		{input: "https://github.com/owner", wantError: true},
		{input: "https://github.com/owner/repo/issues/1", wantError: true},
		{input: "https://github.com/owner/repo/releases/download/v1/App.AppImage", wantError: true},
		{input: "https://example.com/owner/repo", wantError: true},
		{input: "http://github.com/owner/repo", wantError: true},
	}

	for _, tt := range tests {
		got, err := ParsePackageRefURL(tt.input)
		if tt.wantError {
			if err == nil {
				t.Fatalf("ParsePackageRefURL(%q) expected error", tt.input)
			}
			continue
		}
		if err != nil {
			t.Fatalf("ParsePackageRefURL(%q) returned error: %v", tt.input, err)
		}
		if got != tt.expect {
			t.Fatalf("ParsePackageRefURL(%q) = %#v, want %#v", tt.input, got, tt.expect)
		}
	}
}
