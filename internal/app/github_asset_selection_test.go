package app

import (
	"strings"
	"testing"
)

func TestSelectGitHubAppImageAssetForArchSelectsMatchingAMD64Alias(t *testing.T) {
	t.Parallel()

	release := testGitHubRelease(
		"Example-arm64.AppImage",
		"Example-x86_64.AppImage",
		"Example.AppImage.zsync",
		"checksums.txt",
	)

	asset, err := selectGitHubAppImageAssetForArch(release, "amd64")
	if err != nil {
		t.Fatalf("selectGitHubAppImageAssetForArch() error = %v", err)
	}
	if got, want := asset.Name, "Example-x86_64.AppImage"; got != want {
		t.Fatalf("selected asset = %q, want %q", got, want)
	}
}

func TestSelectGitHubAppImageAssetForArchTreatsEquivalentAMD64LabelsAsMatching(t *testing.T) {
	t.Parallel()

	for _, name := range []string{
		"Example-amd64.AppImage",
		"Example-x86_64.AppImage",
		"Example-x86-64.AppImage",
		"Example-x64.AppImage",
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			release := testGitHubRelease(name, "Example-arm64.AppImage")
			asset, err := selectGitHubAppImageAssetForArch(release, "amd64")
			if err != nil {
				t.Fatalf("selectGitHubAppImageAssetForArch() error = %v", err)
			}
			if got, want := asset.Name, name; got != want {
				t.Fatalf("selected asset = %q, want %q", got, want)
			}
		})
	}
}

func TestSelectGitHubAppImageAssetForArchTreatsEquivalentARM64LabelsAsMatching(t *testing.T) {
	t.Parallel()

	for _, name := range []string{
		"Example-arm64.AppImage",
		"Example-aarch64.AppImage",
		"Example-armv8.AppImage",
		"Example-arm-v8.AppImage",
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			release := testGitHubRelease(name, "Example-x86_64.AppImage")
			asset, err := selectGitHubAppImageAssetForArch(release, "arm64")
			if err != nil {
				t.Fatalf("selectGitHubAppImageAssetForArch() error = %v", err)
			}
			if got, want := asset.Name, name; got != want {
				t.Fatalf("selected asset = %q, want %q", got, want)
			}
		})
	}
}

func TestSelectGitHubAppImageAssetForArchTreatsEquivalent386LabelsAsMatching(t *testing.T) {
	t.Parallel()

	for _, name := range []string{
		"Example-386.AppImage",
		"Example-i386.AppImage",
		"Example-i686.AppImage",
		"Example-x86.AppImage",
		"Example-ia32.AppImage",
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			release := testGitHubRelease(name, "Example-x86_64.AppImage")
			asset, err := selectGitHubAppImageAssetForArch(release, "386")
			if err != nil {
				t.Fatalf("selectGitHubAppImageAssetForArch() error = %v", err)
			}
			if got, want := asset.Name, name; got != want {
				t.Fatalf("selected asset = %q, want %q", got, want)
			}
		})
	}
}

func TestSelectGitHubAppImageAssetForArchTreatsEquivalentARMLabelsAsMatching(t *testing.T) {
	t.Parallel()

	for _, name := range []string{
		"Example-arm.AppImage",
		"Example-arm32.AppImage",
		"Example-armhf.AppImage",
		"Example-armv7.AppImage",
		"Example-arm-v7.AppImage",
		"Example-armv7l.AppImage",
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			release := testGitHubRelease(name, "Example-aarch64.AppImage")
			asset, err := selectGitHubAppImageAssetForArch(release, "arm")
			if err != nil {
				t.Fatalf("selectGitHubAppImageAssetForArch() error = %v", err)
			}
			if got, want := asset.Name, name; got != want {
				t.Fatalf("selected asset = %q, want %q", got, want)
			}
		})
	}
}

func TestSelectGitHubAppImageAssetForArchTreatsAdditionalGOARCHLabelsAsMatching(t *testing.T) {
	t.Parallel()

	tests := []struct {
		goarch string
		asset  string
	}{
		{goarch: "riscv64", asset: "Example-riscv64.AppImage"},
		{goarch: "ppc64le", asset: "Example-ppc64le.AppImage"},
		{goarch: "ppc64", asset: "Example-powerpc64.AppImage"},
		{goarch: "s390x", asset: "Example-s390x.AppImage"},
	}

	for _, tt := range tests {
		t.Run(tt.goarch, func(t *testing.T) {
			t.Parallel()

			release := testGitHubRelease(tt.asset, "Example-x86_64.AppImage")
			asset, err := selectGitHubAppImageAssetForArch(release, tt.goarch)
			if err != nil {
				t.Fatalf("selectGitHubAppImageAssetForArch() error = %v", err)
			}
			if got, want := asset.Name, tt.asset; got != want {
				t.Fatalf("selected asset = %q, want %q", got, want)
			}
		})
	}
}

func TestSelectGitHubAppImageAssetForArchPrefersArchSpecificOverGeneric(t *testing.T) {
	t.Parallel()

	release := testGitHubRelease("Example.AppImage", "Example-linux-amd64.AppImage")

	asset, err := selectGitHubAppImageAssetForArch(release, "amd64")
	if err != nil {
		t.Fatalf("selectGitHubAppImageAssetForArch() error = %v", err)
	}
	if got, want := asset.Name, "Example-linux-amd64.AppImage"; got != want {
		t.Fatalf("selected asset = %q, want %q", got, want)
	}
}

func TestSelectGitHubAppImageAssetForArchReturnsOnlyAppImageEvenForDifferentArch(t *testing.T) {
	t.Parallel()

	release := testGitHubRelease("Example-arm64.AppImage")

	asset, err := selectGitHubAppImageAssetForArch(release, "amd64")
	if err != nil {
		t.Fatalf("selectGitHubAppImageAssetForArch() error = %v", err)
	}
	if got, want := asset.Name, "Example-arm64.AppImage"; got != want {
		t.Fatalf("selected asset = %q, want %q", got, want)
	}
}

func TestSelectGitHubAppImageAssetForArchRejectsAmbiguousGenericAssets(t *testing.T) {
	t.Parallel()

	release := testGitHubRelease("Example.AppImage", "Example-Portable.AppImage")

	_, err := selectGitHubAppImageAssetForArch(release, "amd64")
	if err == nil {
		t.Fatal("selectGitHubAppImageAssetForArch() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "multiple matching AppImage assets") {
		t.Fatalf("error = %q, want ambiguity message", err)
	}
}

func TestSelectGitHubAppImageAssetForArchRejectsMissingAppImageAssets(t *testing.T) {
	t.Parallel()

	release := testGitHubRelease("Example.deb", "Example.AppImage.zsync", "checksums.txt")

	_, err := selectGitHubAppImageAssetForArch(release, "amd64")
	if err == nil {
		t.Fatal("selectGitHubAppImageAssetForArch() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "no AppImage assets") {
		t.Fatalf("error = %q, want missing asset message", err)
	}
}

func TestArchitectureLabelsInNameDoesNotTreatX8664As386(t *testing.T) {
	t.Parallel()

	labels := architectureLabelsInName("Example-x86_64.AppImage")
	if !labels[architectureAMD64] {
		t.Fatalf("labels = %#v, want amd64", labels)
	}
	if labels[architecture386] {
		t.Fatalf("labels = %#v, did not want 386", labels)
	}
}

func TestArchitectureLabelsInNameDoesNotTreatARM64AsARM(t *testing.T) {
	t.Parallel()

	labels := architectureLabelsInName("Example-arm64.AppImage")
	if !labels[architectureARM64] {
		t.Fatalf("labels = %#v, want arm64", labels)
	}
	if labels[architectureARM] {
		t.Fatalf("labels = %#v, did not want arm", labels)
	}
}

func testGitHubRelease(assetNames ...string) GitHubRelease {
	assets := make([]GitHubReleaseAsset, 0, len(assetNames))
	for _, name := range assetNames {
		assets = append(assets, GitHubReleaseAsset{
			Name:        name,
			DownloadURL: "https://example.test/" + name,
		})
	}

	return GitHubRelease{
		Repo:    "owner/repo",
		TagName: "v1.2.3",
		Assets:  assets,
	}
}
