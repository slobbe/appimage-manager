package domain

import "testing"

func TestManagedIDCandidates(t *testing.T) {
	candidates := ManagedIDCandidates("Notes", "com.vendor.Notes", "/tmp/Notes.AppImage")
	want := []string{
		"notes",
		"notes-com.vendor.Notes",
		"notes-com.vendor.Notes-" + ShortIdentityHash("com.vendor.Notes", "/tmp/Notes.AppImage"),
	}

	if len(candidates) != len(want) {
		t.Fatalf("len(candidates) = %d, want %d: %#v", len(candidates), len(want), candidates)
	}
	for i := range want {
		if candidates[i] != want[i] {
			t.Fatalf("candidates[%d] = %q, want %q", i, candidates[i], want[i])
		}
	}
}

func TestAppsShareManagedIdentityUsesUpdateSource(t *testing.T) {
	a := &App{
		Update: &UpdateSource{
			Kind: UpdateGitHubRelease,
			GitHubRelease: &GitHubReleaseUpdateSource{
				Repo:  "owner/app",
				Asset: "*.AppImage",
			},
		},
	}
	b := &App{
		Update: &UpdateSource{
			Kind: UpdateGitHubRelease,
			GitHubRelease: &GitHubReleaseUpdateSource{
				Repo:  "owner/app",
				Asset: "*.AppImage",
			},
		},
	}

	if !AppsShareManagedIdentity(a, b) {
		t.Fatal("AppsShareManagedIdentity returned false, want true")
	}
}

func TestAppsShareManagedIdentityUsesSource(t *testing.T) {
	a := &App{
		Source: Source{
			Kind: SourceLocalFile,
			LocalFile: &LocalFileSource{
				OriginalPath: "/tmp/../tmp/App.AppImage",
			},
		},
	}
	b := &App{
		Source: Source{
			Kind: SourceLocalFile,
			LocalFile: &LocalFileSource{
				OriginalPath: "/tmp/App.AppImage",
			},
		},
	}

	if !AppsShareManagedIdentity(a, b) {
		t.Fatal("AppsShareManagedIdentity returned false, want true")
	}
}

func TestUpdateSourcesEqualDoesNotTreatNoneAsIdentity(t *testing.T) {
	a := &UpdateSource{Kind: UpdateNone}
	b := &UpdateSource{Kind: UpdateNone}

	if UpdateSourcesEqual(a, b) {
		t.Fatal("UpdateSourcesEqual returned true, want false")
	}
}
