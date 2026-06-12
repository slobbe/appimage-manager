package domain

import "testing"

func TestNormalizeVersion(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		input  string
		expect string
	}{
		{name: "keeps plain semver", input: "1.2.3", expect: "1.2.3"},
		{name: "trims spaces and v prefix", input: "  v1.2.3  ", expect: "1.2.3"},
		{name: "normalizes version prefix", input: "Version 2.0.0", expect: "2.0.0"},
		{name: "extracts decorated package tag", input: "@standardnotes/desktop@3.201.19", expect: "3.201.19"},
		{name: "extracts release prefix version", input: "release-3.2.1", expect: "3.2.1"},
		{name: "extracts embedded v version", input: "desktop-v1.2.3", expect: "1.2.3"},
		{name: "strips linux arch suffix", input: "1.17.0-linux-x86-64", expect: "1.17.0"},
		{name: "strips arch suffix", input: "1.17.0-x86_64", expect: "1.17.0"},
		{name: "extracts version before underscore arch suffix", input: "Handy_0.8.3_amd64.AppImage", expect: "0.8.3"},
		{name: "extracts version before staged arch hash suffix", input: "handy_0.8.3_amd64-265e144b8aba0ca4", expect: "0.8.3"},
		{name: "keeps prerelease", input: "v1.2.3-rc1", expect: "1.2.3-rc1"},
		{name: "keeps prerelease before packaging suffix", input: "1.17.0-rc1-linux-x86_64", expect: "1.17.0-rc1"},
		{name: "keeps dotted prerelease", input: "foo@1.2.3-beta.1", expect: "1.2.3-beta.1"},
		{name: "uses last matching token", input: "pkg-2024.1@1.2.3", expect: "1.2.3"},
		{name: "clears unknown", input: "unknown", expect: ""},
		{name: "handles empty", input: "", expect: ""},
		{name: "normalizes dashed calendar date", input: "2024-06-12", expect: "2024.06.12"},
		{name: "normalizes compact calendar date", input: "release-20240612", expect: "2024.06.12"},
		{name: "keeps calendar date prerelease", input: "app-2024.06.12-beta.1-x86_64.AppImage", expect: "2024.06.12-beta.1"},
		{name: "normalizes leading zeroes", input: "v01.002.0003", expect: "1.2.3"},
		{name: "keeps build metadata", input: "1.2.3-build.456", expect: "1.2.3+456"},
		{name: "normalizes uppercase prerelease", input: "v1.2.3-BETA.01", expect: "1.2.3-beta.1"},
		{name: "strips appimage zsync extension", input: "Example-2.4.0.AppImage.zsync", expect: "2.4.0"},
		{name: "rejects invalid calendar date", input: "2024-02-31", expect: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := NormalizeVersion(tt.input)
			if tt.expect == "" {
				if ok {
					t.Fatalf("NormalizeVersion(%q) ok = true, want false", tt.input)
				}
				if got != "" {
					t.Fatalf("NormalizeVersion(%q) = %q, want empty", tt.input, got)
				}
				return
			}

			if !ok {
				t.Fatalf("NormalizeVersion(%q) ok = false, want true", tt.input)
			}
			if got != tt.expect {
				t.Fatalf("NormalizeVersion(%q) = %q, want %q", tt.input, got, tt.expect)
			}
		})
	}
}

func TestParseVersionKeepsRawCandidate(t *testing.T) {
	t.Parallel()

	version, ok := ParseVersion("desktop-v1.2.3-beta.1-x86_64.AppImage")
	if !ok {
		t.Fatal("ParseVersion() ok = false, want true")
	}

	if got, want := version.String(), "1.2.3-beta.1"; got != want {
		t.Fatalf("Version.String() = %q, want %q", got, want)
	}

	if got, want := version.Raw(), "1.2.3-beta.1-x86_64.AppImage"; got != want {
		t.Fatalf("Version.Raw() = %q, want %q", got, want)
	}

	if version.IsZero() {
		t.Fatal("Version.IsZero() = true, want false")
	}
}

func TestZeroVersion(t *testing.T) {
	t.Parallel()

	var version Version
	if !version.IsZero() {
		t.Fatal("zero Version IsZero() = false, want true")
	}
	if got := version.String(); got != "" {
		t.Fatalf("zero Version String() = %q, want empty", got)
	}
	if got := version.Raw(); got != "" {
		t.Fatalf("zero Version Raw() = %q, want empty", got)
	}
}

func TestCompareVersions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		left   string
		right  string
		expect int
	}{
		{name: "equal versions", left: "1.2.3", right: "1.2.3", expect: 0},
		{name: "ignores missing trailing zero", left: "1.2", right: "1.2.0", expect: 0},
		{name: "newer patch", left: "1.2.4", right: "1.2.3", expect: 1},
		{name: "older patch", left: "1.2.3", right: "1.2.4", expect: -1},
		{name: "newer minor", left: "1.3.0", right: "1.2.9", expect: 1},
		{name: "newer major", left: "2.0.0", right: "1.99.99", expect: 1},
		{name: "calendar date newer day", left: "2024.06.12", right: "2024.06.11", expect: 1},
		{name: "calendar date older month", left: "2024.05.31", right: "2024.06.01", expect: -1},
		{name: "stable is newer than beta", left: "1.2.3", right: "1.2.3-beta.1", expect: 1},
		{name: "beta is older than stable", left: "1.2.3-beta.1", right: "1.2.3", expect: -1},
		{name: "newer alpha beats older stable", left: "1.2.3-alpha.1", right: "1.2.2", expect: 1},
		{name: "newer beta beats older stable", left: "1.2.3-beta.1", right: "1.2.2", expect: 1},
		{name: "newer pre beats older stable", left: "1.2.3-pre.1", right: "1.2.2", expect: 1},
		{name: "newer preview beats older stable", left: "1.2.3-preview.1", right: "1.2.2", expect: 1},
		{name: "newer rc beats older stable", left: "1.2.3-rc.1", right: "1.2.2", expect: 1},
		{name: "newer nightly beats older stable", left: "1.2.3-nightly.20240612", right: "1.2.2", expect: 1},
		{name: "newer snapshot beats older stable", left: "1.2.3-snapshot.1", right: "1.2.2", expect: 1},
		{name: "newer unknown prerelease beats older stable", left: "1.2.3-exp.1", right: "1.2.2", expect: 1},
		{name: "older prerelease loses to newer stable", left: "1.2.2-rc.9", right: "1.2.3", expect: -1},
		{name: "newer beta number", left: "1.2.3-beta.2", right: "1.2.3-beta.1", expect: 1},
		{name: "rc is newer than beta", left: "1.2.3-rc.1", right: "1.2.3-beta.9", expect: 1},
		{name: "rc suffix number compares", left: "1.2.3-rc2", right: "1.2.3-rc1", expect: 1},
		{name: "alpha is older than beta", left: "1.2.3-alpha.9", right: "1.2.3-beta.1", expect: -1},
		{name: "longer prerelease is newer when prefix equal", left: "1.2.3-alpha.1", right: "1.2.3-alpha", expect: 1},
		{name: "numeric prerelease identifiers compare numerically", left: "1.2.3-beta.10", right: "1.2.3-beta.2", expect: 1},
		{name: "build metadata does not affect precedence", left: "1.2.3+456", right: "1.2.3+123", expect: 0},
		{name: "build metadata after prerelease does not affect precedence", left: "1.2.3-beta.1+456", right: "1.2.3-beta.1+123", expect: 0},
		{name: "date prerelease is older than date stable", left: "2024.06.12-beta.1", right: "2024.06.12", expect: -1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := CompareVersions(tt.left, tt.right); got != tt.expect {
				t.Fatalf("CompareVersions(%q, %q) = %d, want %d", tt.left, tt.right, got, tt.expect)
			}
		})
	}
}
