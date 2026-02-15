package core

import "testing"

func TestNormalizeVersion(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		expect string
	}{
		{name: "keeps plain semver", input: "1.2.3", expect: "1.2.3"},
		{name: "trims spaces and v prefix", input: "  v1.2.3  ", expect: "1.2.3"},
		{name: "normalizes version prefix", input: "Version 2.0.0", expect: "2.0.0"},
		{name: "handles empty", input: "", expect: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeVersion(tt.input)
			if got != tt.expect {
				t.Fatalf("normalizeVersion(%q) = %q, want %q", tt.input, got, tt.expect)
			}
		})
	}
}

func TestSelectRelease(t *testing.T) {
	releases := []gitHubReleaseResponse{
		{TagName: "v3.0.0", Draft: true},
		{TagName: "v2.0.0-rc1", Prerelease: true},
		{TagName: "v1.0.0", Prerelease: false},
	}

	gotStable, okStable := selectRelease(releases, false)
	if !okStable {
		t.Fatal("selectRelease returned no result for stable selection")
	}
	if gotStable.TagName != "v1.0.0" {
		t.Fatalf("stable selectRelease picked %q, want %q", gotStable.TagName, "v1.0.0")
	}

	gotPre, okPre := selectRelease(releases, true)
	if !okPre {
		t.Fatal("selectRelease returned no result for prerelease selection")
	}
	if gotPre.TagName != "v2.0.0-rc1" {
		t.Fatalf("prerelease selectRelease picked %q, want %q", gotPre.TagName, "v2.0.0-rc1")
	}

	_, okNone := selectRelease([]gitHubReleaseResponse{{TagName: "v1", Draft: true}}, false)
	if okNone {
		t.Fatal("selectRelease should return no match when only drafts are present")
	}
}

func TestMatchAssetArchPreference(t *testing.T) {
	assets := []releaseAsset{
		{Name: "MyApp-arm64.AppImage", BrowserDownloadURL: "https://example.com/arm64"},
		{Name: "MyApp.AppImage", BrowserDownloadURL: "https://example.com/generic"},
		{Name: "MyApp-x86_64.AppImage", BrowserDownloadURL: "https://example.com/amd64"},
	}

	nameAMD64, urlAMD64 := matchAsset(assets, "*.AppImage", "amd64")
	if nameAMD64 != "MyApp-x86_64.AppImage" || urlAMD64 != "https://example.com/amd64" {
		t.Fatalf("amd64 selection got (%q, %q), want (%q, %q)", nameAMD64, urlAMD64, "MyApp-x86_64.AppImage", "https://example.com/amd64")
	}

	nameARM64, urlARM64 := matchAsset(assets, "*.AppImage", "arm64")
	if nameARM64 != "MyApp-arm64.AppImage" || urlARM64 != "https://example.com/arm64" {
		t.Fatalf("arm64 selection got (%q, %q), want (%q, %q)", nameARM64, urlARM64, "MyApp-arm64.AppImage", "https://example.com/arm64")
	}

	nameUnknown, urlUnknown := matchAsset(assets, "*.AppImage", "riscv64")
	if nameUnknown != "MyApp.AppImage" || urlUnknown != "https://example.com/generic" {
		t.Fatalf("unknown-arch selection got (%q, %q), want (%q, %q)", nameUnknown, urlUnknown, "MyApp.AppImage", "https://example.com/generic")
	}
}

func TestMatchAssetNoMatch(t *testing.T) {
	assets := []releaseAsset{
		{Name: "MyApp-x86_64.AppImage", BrowserDownloadURL: "https://example.com/amd64"},
	}

	name, url := matchAsset(assets, "*.zsync", "amd64")
	if name != "" || url != "" {
		t.Fatalf("matchAsset should return empty result for non-matching pattern, got (%q, %q)", name, url)
	}
}
