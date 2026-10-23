package app

import (
	"path/filepath"
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

func TestValidateGitHubAssetPattern(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name    string
		pattern string
		match   string
		invalid bool
	}{
		{name: "empty"},
		{name: "consecutive stars", pattern: "prefix**suffix", match: "prefix-middle-suffix"},
		{name: "escaped open outside class", pattern: `prefix\[*suffix`, match: "prefix[anything-suffix"},
		{name: "escaped close outside class", pattern: `prefix\]*suffix`, match: "prefix]anything-suffix"},
		{name: "escaped star", pattern: `prefix\*suffix`, match: "prefix*suffix"},
		{name: "escaped backslash before star", pattern: `prefix\\*suffix`, match: `prefix\anything-suffix`},
		{name: "literal open inside class", pattern: `prefix[[]*suffix`, match: "prefix[anything-suffix"},
		{name: "star and open inside class", pattern: `prefix[*[]*suffix`, match: "prefix*anything-suffix"},
		{name: "escaped open inside class", pattern: `prefix[\[]*suffix`, match: "prefix[anything-suffix"},
		{name: "escaped close inside class", pattern: `prefix[\]]*suffix`, match: "prefix]anything-suffix"},
		{name: "escaped close then star inside class", pattern: `prefix[\]*]*suffix`, match: "prefix*anything-suffix"},
		{name: "escaped backslash inside class", pattern: `prefix[\\]*suffix`, match: `prefix\anything-suffix`},
		{name: "negated class with escaped close", pattern: `prefix[^\]]*suffix`, match: "prefix-anything-suffix"},
		{name: "escaped delimiters in range", pattern: `prefix[\[-\]]*suffix`, match: "prefix\\anything-suffix"},
		{name: "malformed later chunk", pattern: "prefix*[", invalid: true},
		{name: "malformed third chunk", pattern: "prefix*middle*[", invalid: true},
		{name: "empty class", pattern: "prefix*[]", invalid: true},
		{name: "unterminated escaped close", pattern: `prefix*[\]`, invalid: true},
		{name: "unterminated escaped open", pattern: `prefix*[\[`, invalid: true},
		{name: "unterminated range", pattern: `prefix*[a-]`, invalid: true},
		{name: "escaped open then malformed chunk", pattern: `prefix\[*[`, invalid: true},
		{name: "escaped close then malformed chunk", pattern: `prefix\]*[`, invalid: true},
		{name: "escaped backslash then malformed chunk", pattern: `prefix\\*[`, invalid: true},
		{name: "escaped class close and star then malformed chunk", pattern: `prefix[\]*]*[`, invalid: true},
		{name: "escaped class open then malformed chunk", pattern: `prefix[\[]*[`, invalid: true},
		{name: "escaped class backslash then malformed chunk", pattern: `prefix[\\]*[`, invalid: true},
		{name: "trailing escape after star", pattern: `prefix*\`, invalid: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := validateGitHubAssetPattern(tt.pattern)
			if tt.invalid {
				if err != filepath.ErrBadPattern {
					t.Fatalf("validateGitHubAssetPattern(%q) = %v, want ErrBadPattern", tt.pattern, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("validateGitHubAssetPattern(%q) = %v", tt.pattern, err)
			}
			if matched, err := filepath.Match(tt.pattern, tt.match); err != nil || !matched {
				t.Fatalf("filepath.Match(%q, %q) = %v, %v, want true, nil", tt.pattern, tt.match, matched, err)
			}
		})
	}
}

func TestValidateGitHubAssetSelection(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		selection githubAssetSelection
		wantError string
	}{
		{name: "defaults"},
		{name: "blank optional pattern", selection: githubAssetSelection{pattern: " \t "}},
		{
			name: "valid trimmed globs",
			selection: githubAssetSelection{
				pattern: " *.AppImage ", excludePatterns: []string{" *debug* ", "*[ab]?*"}, preferPatterns: []string{" *linux* ", "*stable*"},
			},
		},
		{name: "stars in character class", selection: githubAssetSelection{pattern: "prefix[*[]suffix*"}},
		{name: "escaped star and bracket", selection: githubAssetSelection{pattern: `prefix\*\[suffix*`}},
		{name: "escaped bracket in character class", selection: githubAssetSelection{pattern: `prefix[\]*]suffix*`}},
		{name: "invalid asset", selection: githubAssetSelection{pattern: " [ "}, wantError: "invalid GitHub asset pattern"},
		{name: "invalid trailing escape", selection: githubAssetSelection{pattern: "prefix*\\"}, wantError: "invalid GitHub asset pattern"},
		{name: "invalid asset suffix", selection: githubAssetSelection{pattern: "no-match*["}, wantError: "invalid GitHub asset pattern"},
		{name: "blank exclude", selection: githubAssetSelection{excludePatterns: []string{""}}, wantError: "exclude pattern must not be blank"},
		{name: "whitespace exclude", selection: githubAssetSelection{excludePatterns: []string{"*", " \t "}}, wantError: "exclude pattern must not be blank"},
		{name: "invalid later exclude", selection: githubAssetSelection{excludePatterns: []string{"*", "["}}, wantError: "invalid GitHub asset exclude pattern"},
		{name: "invalid exclude suffix", selection: githubAssetSelection{excludePatterns: []string{"no-match*["}}, wantError: "invalid GitHub asset exclude pattern"},
		{name: "blank prefer", selection: githubAssetSelection{preferPatterns: []string{""}}, wantError: "prefer pattern must not be blank"},
		{name: "whitespace prefer", selection: githubAssetSelection{preferPatterns: []string{"*", " \n "}}, wantError: "prefer pattern must not be blank"},
		{name: "invalid later prefer", selection: githubAssetSelection{preferPatterns: []string{"*", "["}}, wantError: "invalid GitHub asset prefer pattern"},
		{name: "invalid prefer suffix", selection: githubAssetSelection{preferPatterns: []string{"no-match*["}}, wantError: "invalid GitHub asset prefer pattern"},
		{
			name:      "validate prefer even when exclusions remove everything",
			selection: githubAssetSelection{excludePatterns: []string{"*"}, preferPatterns: []string{"["}},
			wantError: "invalid GitHub asset prefer pattern",
		},
		{
			name:      "validate exclude even when asset pattern cannot match",
			selection: githubAssetSelection{pattern: "missing*", excludePatterns: []string{"["}},
			wantError: "invalid GitHub asset exclude pattern",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := validateGitHubAssetSelection(tt.selection)
			if tt.wantError == "" {
				if err != nil {
					t.Fatalf("validateGitHubAssetSelection() error = %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantError) {
				t.Fatalf("validateGitHubAssetSelection() error = %v, want %q", err, tt.wantError)
			}
			for _, release := range []GitHubRelease{
				testGitHubRelease(),
				testGitHubRelease("checksums.txt"),
				testGitHubRelease("Example.AppImage"),
				testGitHubRelease("Example.AppImage", "Other.AppImage"),
			} {
				asset, selectionErr := selectGitHubAppImageAssetWithSelection(release, "amd64", tt.selection)
				if selectionErr == nil || selectionErr.Error() != err.Error() {
					t.Fatalf("selection with %d assets error = %v, want %v", len(release.Assets), selectionErr, err)
				}
				if asset != (GitHubReleaseAsset{}) {
					t.Fatalf("asset on validation failure = %#v, want zero value", asset)
				}
			}
		})
	}
}

func TestSelectGitHubAppImageAssetWithSelection(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		assets    []string
		selection githubAssetSelection
		want      string
		wantError string
	}{
		{
			name:   "default architecture and linux scoring",
			assets: []string{"Example.AppImage", "Example-amd64.AppImage", "Example-linux-amd64.AppImage", "Example-linux-arm64.AppImage"},
			want:   "Example-linux-amd64.AppImage",
		},
		{
			name:   "only mismatched architecture still selected",
			assets: []string{"Example-arm64.AppImage", "Example.AppImage.zsync"},
			want:   "Example-arm64.AppImage",
		},
		{
			name:   "generic beats wrong architecture",
			assets: []string{"Example.AppImage", "Example-linux-arm64.AppImage"},
			want:   "Example.AppImage",
		},
		{
			name:      "blank optional pattern retains scoring",
			assets:    []string{"Example.AppImage", "Example-amd64.AppImage"},
			selection: githubAssetSelection{pattern: " \t "}, want: "Example-amd64.AppImage",
		},
		{
			name:   "case insensitive appimage suffix",
			assets: []string{"Example.APPIMAGE", "Example.AppImage.zsync", "checksums.txt"},
			want:   "Example.APPIMAGE",
		},
		{
			name:      "explicit pattern bypasses architecture",
			assets:    []string{"Example-amd64.AppImage", "Example-arm64.AppImage"},
			selection: githubAssetSelection{pattern: " *arm64.AppImage "}, want: "Example-arm64.AppImage",
		},
		{
			name:      "broad explicit pattern bypasses all scoring",
			assets:    []string{"Example.AppImage", "Example-linux-amd64.AppImage"},
			selection: githubAssetSelection{pattern: "*"},
			wantError: `release owner/repo@v1.2.3 has multiple AppImage assets matching pattern "*": Example-linux-amd64.AppImage, Example.AppImage`,
		},
		{
			name:      "non appimage cannot be explicitly selected",
			assets:    []string{"Example.AppImage", "Example.deb"},
			selection: githubAssetSelection{pattern: "*.deb"},
			wantError: `release owner/repo@v1.2.3 has no AppImage assets matching pattern "*.deb"`,
		},
		{
			name:      "exclude before architecture scoring",
			assets:    []string{"Example-linux-amd64.AppImage", "Example.AppImage", "Example-arm64.AppImage"},
			selection: githubAssetSelection{excludePatterns: []string{"*amd64*"}}, want: "Example.AppImage",
		},
		{
			name:      "exclusions use OR and trim patterns",
			assets:    []string{"Example-debug-amd64.AppImage", "Example-beta-amd64.AppImage", "Example.AppImage"},
			selection: githubAssetSelection{excludePatterns: []string{" *debug* ", " *beta* "}}, want: "Example.AppImage",
		},
		{
			name:      "explicit pattern then exclusions then preferences",
			assets:    []string{"Other.AppImage", "Example-arm64.AppImage", "Example-amd64.AppImage"},
			selection: githubAssetSelection{pattern: "Example*", excludePatterns: []string{"*amd64*"}, preferPatterns: []string{"Other*", "*amd64*"}},
			want:      "Example-arm64.AppImage",
		},
		{
			name:      "preference breaks architecture score tie",
			assets:    []string{"Example-stable-amd64.AppImage", "Example-beta-amd64.AppImage", "Example.AppImage"},
			selection: githubAssetSelection{preferPatterns: []string{" *stable* "}}, want: "Example-stable-amd64.AppImage",
		},
		{
			name:      "preference cannot override architecture score",
			assets:    []string{"Example-arm64.AppImage", "Example-amd64.AppImage"},
			selection: githubAssetSelection{preferPatterns: []string{"*arm64*"}}, want: "Example-amd64.AppImage",
		},
		{
			name:      "preference cannot override linux score",
			assets:    []string{"Example-stable-amd64.AppImage", "Example-linux-amd64.AppImage"},
			selection: githubAssetSelection{preferPatterns: []string{"*stable*"}}, want: "Example-linux-amd64.AppImage",
		},
		{
			name:      "explicit pattern preferences bypass architecture",
			assets:    []string{"Example-amd64.AppImage", "Example-arm64.AppImage"},
			selection: githubAssetSelection{pattern: "*.AppImage", preferPatterns: []string{"*arm64*"}}, want: "Example-arm64.AppImage",
		},
		{
			name:      "preferences OR not priority or intersection",
			assets:    []string{"C.AppImage", "B.AppImage", "A.AppImage"},
			selection: githubAssetSelection{preferPatterns: []string{"B*", "A*"}},
			wantError: `release owner/repo@v1.2.3 has multiple matching AppImage assets for architecture "amd64": A.AppImage, B.AppImage`,
		},
		{
			name:      "later preference can match",
			assets:    []string{"A.AppImage", "B.AppImage"},
			selection: githubAssetSelection{preferPatterns: []string{"missing*", "B*"}}, want: "B.AppImage",
		},
		{
			name:      "unmatched preferences fall back to ambiguity",
			assets:    []string{"B.AppImage", "A.AppImage"},
			selection: githubAssetSelection{preferPatterns: []string{"missing*"}},
			wantError: `release owner/repo@v1.2.3 has multiple matching AppImage assets for architecture "amd64": A.AppImage, B.AppImage`,
		},
		{
			name:      "overlapping preferences do not duplicate assets",
			assets:    []string{"A.AppImage", "B.AppImage"},
			selection: githubAssetSelection{preferPatterns: []string{"A*", "A.AppImage"}}, want: "A.AppImage",
		},
		{
			name:      "no matches after exclusions",
			assets:    []string{"Example.AppImage"},
			selection: githubAssetSelection{excludePatterns: []string{"*"}, preferPatterns: []string{"*"}},
			wantError: "release owner/repo@v1.2.3 has no AppImage assets remaining after exclusions",
		},
		{
			name:      "explicit matches all excluded",
			assets:    []string{"Example.AppImage", "Other.AppImage"},
			selection: githubAssetSelection{pattern: "Example*", excludePatterns: []string{"Example*"}},
			wantError: "release owner/repo@v1.2.3 has no AppImage assets remaining after exclusions",
		},
		{
			name:      "no initial glob match is not exclusion failure",
			assets:    []string{"Example.AppImage"},
			selection: githubAssetSelection{pattern: "missing*", excludePatterns: []string{"*"}},
			wantError: `release owner/repo@v1.2.3 has no AppImage assets matching pattern "missing*"`,
		},
		{
			name:      "no assets",
			wantError: "release owner/repo@v1.2.3 has no AppImage assets",
		},
		{
			name:      "no appimages with filters",
			assets:    []string{"Example.deb", "Example.AppImage.zsync"},
			selection: githubAssetSelection{excludePatterns: []string{"*"}, preferPatterns: []string{"*"}},
			wantError: "release owner/repo@v1.2.3 has no AppImage assets",
		},
		{
			name:      "ambiguity lists only highest score remaining candidates",
			assets:    []string{"Z-amd64.AppImage", "A-amd64.AppImage", "Generic.AppImage", "Excluded-amd64.AppImage"},
			selection: githubAssetSelection{excludePatterns: []string{"Excluded*"}},
			wantError: `release owner/repo@v1.2.3 has multiple matching AppImage assets for architecture "amd64": A-amd64.AppImage, Z-amd64.AppImage`,
		},
		{
			name:      "explicit ambiguity lists only preferred candidates",
			assets:    []string{"Z-amd64.AppImage", "A-arm64.AppImage", "Generic.AppImage", "Excluded-amd64.AppImage"},
			selection: githubAssetSelection{pattern: "*", excludePatterns: []string{"Excluded*"}, preferPatterns: []string{"*-amd64*", "*-arm64*"}},
			wantError: `release owner/repo@v1.2.3 has multiple AppImage assets matching pattern "*": A-arm64.AppImage, Z-amd64.AppImage`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			// Rotate and reverse release order to exercise score and filter independence.
			for order := 0; order < 2*(len(tt.assets)+1); order++ {
				names := append([]string(nil), tt.assets...)
				if len(names) > 0 {
					offset := (order / 2) % len(names)
					names = append(names[offset:], names[:offset]...)
				}
				if order%2 != 0 {
					for i, j := 0, len(names)-1; i < j; i, j = i+1, j-1 {
						names[i], names[j] = names[j], names[i]
					}
				}
				release := testGitHubRelease(names...)
				asset, err := selectGitHubAppImageAssetWithSelection(release, "amd64", tt.selection)
				if tt.wantError != "" {
					if err == nil || err.Error() != tt.wantError {
						t.Fatalf("order %v: error = %v, want %q", names, err, tt.wantError)
					}
					if asset != (GitHubReleaseAsset{}) {
						t.Fatalf("asset on failure = %#v, want zero value", asset)
					}
					continue
				}
				if err != nil {
					t.Fatalf("order %v: error = %v", names, err)
				}
				want := testGitHubRelease(tt.want).Assets[0]
				if asset != want {
					t.Fatalf("asset = %#v, want %#v", asset, want)
				}
				for i, original := range release.Assets {
					if original.Name != names[i] {
						t.Fatalf("release assets mutated: %v", release.Assets)
					}
				}
			}
		})
	}
}

func TestSelectGitHubAppImageAssetMatchingPattern(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name      string
		assets    []string
		pattern   string
		want      string
		wantError string
	}{
		{name: "trimmed explicit selection", assets: []string{"Example-arm64.AppImage", "Other.AppImage"}, pattern: " *arm64* ", want: "Example-arm64.AppImage"},
		{name: "invalid without assets", pattern: "[", wantError: "invalid GitHub asset pattern"},
		{name: "invalid with one asset", assets: []string{"Example.AppImage"}, pattern: "[", wantError: "invalid GitHub asset pattern"},
		{name: "no match", assets: []string{"Example.AppImage"}, pattern: "missing*", wantError: "no AppImage assets matching pattern"},
		{name: "ambiguous", assets: []string{"Example.AppImage", "Other.AppImage"}, pattern: "*", wantError: "multiple AppImage assets matching pattern"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			asset, err := selectGitHubAppImageAssetMatchingPattern(testGitHubRelease(tt.assets...), tt.pattern)
			if tt.wantError != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantError) {
					t.Fatalf("error = %v, want %q", err, tt.wantError)
				}
				return
			}
			if err != nil || asset.Name != tt.want {
				t.Fatalf("asset = %q, error = %v, want %q", asset.Name, err, tt.want)
			}
		})
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
