package domain

import "testing"

func TestNormalizeComparableVersion(t *testing.T) {
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
		{name: "keeps prerelease", input: "v1.2.3-rc1", expect: "1.2.3-rc1"},
		{name: "keeps prerelease before packaging suffix", input: "1.17.0-rc1-linux-x86_64", expect: "1.17.0-rc1"},
		{name: "keeps dotted prerelease", input: "foo@1.2.3-beta.1", expect: "1.2.3-beta.1"},
		{name: "uses last matching token", input: "pkg-2024.1@1.2.3", expect: "1.2.3"},
		{name: "clears unknown", input: "unknown", expect: ""},
		{name: "handles empty", input: "", expect: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NormalizeComparableVersion(tt.input)
			if got != tt.expect {
				t.Fatalf("NormalizeComparableVersion(%q) = %q, want %q", tt.input, got, tt.expect)
			}
		})
	}
}

func TestReleaseAvailability(t *testing.T) {
	latest, available := ReleaseAvailability("1.0.0", "v1.1.0")
	if latest != "1.1.0" || !available {
		t.Fatalf("ReleaseAvailability returned %q, %v; want 1.1.0, true", latest, available)
	}

	latest, available = ReleaseAvailability("1.1.0-linux-x86_64", "v1.1.0")
	if latest != "1.1.0" || available {
		t.Fatalf("ReleaseAvailability returned %q, %v; want 1.1.0, false", latest, available)
	}
}

func TestNormalizeSelfUpdateVersion(t *testing.T) {
	if got := NormalizeSelfUpdateVersion("dev"); got != "" {
		t.Fatalf("NormalizeSelfUpdateVersion(dev) = %q, want empty", got)
	}
	if got := NormalizeSelfUpdateVersion("v1.2.3"); got != "1.2.3" {
		t.Fatalf("NormalizeSelfUpdateVersion(v1.2.3) = %q, want 1.2.3", got)
	}
}

func TestCompareSelfUpdateVersionsHonorsPrereleaseOrdering(t *testing.T) {
	tests := []struct {
		name   string
		left   string
		right  string
		expect int
	}{
		{name: "newer prerelease numeric identifier", left: "0.17.1-pre.3", right: "0.17.1-pre.2", expect: 1},
		{name: "older prerelease numeric identifier", left: "0.17.1-pre.2", right: "0.17.1-pre.3", expect: -1},
		{name: "stable is newer than prerelease", left: "0.17.1", right: "0.17.1-pre.3", expect: 1},
		{name: "prerelease is older than stable", left: "0.17.1-pre.3", right: "0.17.1", expect: -1},
		{name: "numeric identifiers compare numerically", left: "0.17.1-rc.10", right: "0.17.1-rc.2", expect: 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := CompareSelfUpdateVersions(tt.left, tt.right)
			if err != nil {
				t.Fatalf("CompareSelfUpdateVersions returned error: %v", err)
			}
			if got != tt.expect {
				t.Fatalf("CompareSelfUpdateVersions(%q, %q) = %d, want %d", tt.left, tt.right, got, tt.expect)
			}
		})
	}
}

func TestCompareComparableVersions(t *testing.T) {
	tests := []struct {
		name    string
		left    string
		right   string
		expect  int
		wantErr bool
	}{
		{name: "newer", left: "0.12.5", right: "0.12.4", expect: 1},
		{name: "same", left: "0.12.5", right: "0.12.5", expect: 0},
		{name: "older", left: "0.12.4", right: "0.12.5", expect: -1},
		{name: "ignores prerelease suffix", left: "0.12.5-rc1", right: "0.12.5", expect: 0},
		{name: "invalid", left: "dev", right: "0.12.5", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := CompareComparableVersions(tt.left, tt.right)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("CompareComparableVersions returned error: %v", err)
			}
			if got != tt.expect {
				t.Fatalf("CompareComparableVersions(%q, %q) = %d, want %d", tt.left, tt.right, got, tt.expect)
			}
		})
	}
}
