package github

import (
	"strings"
	"testing"
)

func TestSelectRelease(t *testing.T) {
	releases := []releaseResponse{
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

	_, okNone := selectRelease([]releaseResponse{{TagName: "v1", Draft: true}}, false)
	if okNone {
		t.Fatal("selectRelease should return no match when only drafts are present")
	}
}

func TestMatchAssetArchPreference(t *testing.T) {
	assets := []apiAsset{
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

func TestMatchAssetTreatsForeignArchAsNonGeneric(t *testing.T) {
	assets := []apiAsset{
		{Name: "YouTube-Music-3.11.0-armv7l.AppImage", BrowserDownloadURL: "https://example.com/armv7l"},
		{Name: "YouTube-Music-3.11.0.AppImage", BrowserDownloadURL: "https://example.com/generic"},
		{Name: "YouTube-Music-3.11.0-arm64.AppImage", BrowserDownloadURL: "https://example.com/arm64"},
	}

	name, url := matchAsset(assets, "*.AppImage", "amd64")
	if name != "YouTube-Music-3.11.0.AppImage" || url != "https://example.com/generic" {
		t.Fatalf("selection got (%q, %q), want generic AppImage", name, url)
	}
}

func TestMatchAssetAdditionalArchAliases(t *testing.T) {
	tests := []struct {
		name      string
		arch      string
		assetName string
	}{
		{name: "arm64 aarch64", arch: "arm64", assetName: "MyApp-aarch64.AppImage"},
		{name: "arm armv7l", arch: "arm", assetName: "MyApp-armv7l.AppImage"},
		{name: "amd64 x86-64", arch: "amd64", assetName: "MyApp-x86-64.AppImage"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assets := []apiAsset{
				{Name: "MyApp.AppImage", BrowserDownloadURL: "https://example.com/generic"},
				{Name: tt.assetName, BrowserDownloadURL: "https://example.com/match"},
			}
			name, url := matchAsset(assets, "*.AppImage", tt.arch)
			if name != tt.assetName || url != "https://example.com/match" {
				t.Fatalf("selection got (%q, %q), want (%q, %q)", name, url, tt.assetName, "https://example.com/match")
			}
		})
	}
}

func TestMatchAssetOnlyForeignArchIsAmbiguous(t *testing.T) {
	assets := []apiAsset{
		{Name: "MyApp-arm64.AppImage", BrowserDownloadURL: "https://example.com/arm64"},
		{Name: "MyApp-armv7l.AppImage", BrowserDownloadURL: "https://example.com/armv7l"},
	}

	selection := matchAssetSelection(assets, "*.AppImage", "amd64")
	if !selection.ambiguous {
		t.Fatal("expected only foreign assets to be ambiguous")
	}
	if !strings.Contains(selection.reason, "no asset matches local architecture amd64") {
		t.Fatalf("reason = %q", selection.reason)
	}
}

func TestMatchAssetMultipleLocalArchIsAmbiguous(t *testing.T) {
	assets := []apiAsset{
		{Name: "MyApp-x86_64.AppImage", BrowserDownloadURL: "https://example.com/x86_64"},
		{Name: "MyApp-amd64.AppImage", BrowserDownloadURL: "https://example.com/amd64"},
	}

	selection := matchAssetSelection(assets, "*.AppImage", "amd64")
	if !selection.ambiguous {
		t.Fatal("expected multiple local arch assets to be ambiguous")
	}
	if len(selection.candidates) != 2 {
		t.Fatalf("candidate count = %d, want 2", len(selection.candidates))
	}
}

func TestMatchAssetMultipleGenericIsAmbiguous(t *testing.T) {
	assets := []apiAsset{
		{Name: "MyApp.AppImage", BrowserDownloadURL: "https://example.com/one"},
		{Name: "MyApp-portable.AppImage", BrowserDownloadURL: "https://example.com/two"},
	}

	selection := matchAssetSelection(assets, "*.AppImage", "amd64")
	if !selection.ambiguous {
		t.Fatal("expected multiple generic assets to be ambiguous")
	}
}

func TestClassifyAssetArchAvoidsSubstringFalsePositive(t *testing.T) {
	arch, label := classifyAssetArch("Charmingly.Named.AppImage")
	if arch != "" || label != "generic" {
		t.Fatalf("classifyAssetArch returned (%q, %q), want generic", arch, label)
	}
}

func TestMatchAssetNoMatch(t *testing.T) {
	assets := []apiAsset{
		{Name: "MyApp-x86_64.AppImage", BrowserDownloadURL: "https://example.com/amd64"},
	}

	name, url := matchAsset(assets, "*.zsync", "amd64")
	if name != "" || url != "" {
		t.Fatalf("matchAsset should return empty result for non-matching pattern, got (%q, %q)", name, url)
	}
}
