package domain

import "testing"

func TestValidateUpdateSource(t *testing.T) {
	valid := &UpdateSource{
		Kind: UpdateGitHubRelease,
		GitHubRelease: &GitHubReleaseUpdateSource{
			Repo:  "owner/repo",
			Asset: "*.AppImage",
		},
	}
	if err := ValidateUpdateSource(valid); err != nil {
		t.Fatalf("ValidateUpdateSource returned error: %v", err)
	}

	invalid := &UpdateSource{Kind: UpdateGitHubRelease, GitHubRelease: &GitHubReleaseUpdateSource{Repo: "owner/repo"}}
	if err := ValidateUpdateSource(invalid); err == nil {
		t.Fatal("ValidateUpdateSource accepted missing asset")
	}
}

func TestNewReleaseUpdate(t *testing.T) {
	update := NewReleaseUpdate("1.0.0", "v1.1.0", "https://example.com/app.AppImage", "app.AppImage", true, ReleaseTransport{
		Transport:    "zsync",
		ZsyncURL:     "https://example.com/app.AppImage.zsync",
		ExpectedSHA1: "abc",
	})

	if !update.Available {
		t.Fatal("NewReleaseUpdate Available = false, want true")
	}
	if update.LatestVersion != "1.1.0" {
		t.Fatalf("LatestVersion = %q, want 1.1.0", update.LatestVersion)
	}
	if update.AvailabilityLabel != "Pre-release update available" {
		t.Fatalf("AvailabilityLabel = %q", update.AvailabilityLabel)
	}
	if update.Transport != "zsync" || update.ZsyncURL == "" || update.ExpectedSHA1 != "abc" {
		t.Fatalf("transport fields not preserved: %+v", update)
	}
}

func TestSelectReleaseTransport(t *testing.T) {
	selected := SelectReleaseTransport(&ReleaseTransport{
		Transport:    " zsync ",
		ZsyncURL:     " https://example.com/app.zsync ",
		ExpectedSHA1: " abc ",
	}, nil)
	if selected.Transport != "zsync" || selected.ZsyncURL != "https://example.com/app.zsync" || selected.ExpectedSHA1 != "abc" {
		t.Fatalf("SelectReleaseTransport = %+v", selected)
	}

	if selected := SelectReleaseTransport(&ReleaseTransport{Transport: "zsync"}, assertErr{}); selected.Transport != "" {
		t.Fatalf("SelectReleaseTransport with error = %+v, want empty", selected)
	}
}

type assertErr struct{}

func (assertErr) Error() string { return "probe failed" }
