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

func TestResolveManagedAppIdentityReturnsReplacementForEquivalentDifferentID(t *testing.T) {
	incoming := &App{
		ID:   "new.desktop",
		Name: "New Name",
		Update: &UpdateSource{
			Kind: UpdateZsync,
			Zsync: &ZsyncUpdateSource{
				UpdateInfo: "zsync|https://example.com/app.zsync",
				Transport:  "zsync",
			},
		},
	}
	existing := map[string]*App{
		"old-id": {
			ID: "old-id",
			Update: &UpdateSource{
				Kind: UpdateZsync,
				Zsync: &ZsyncUpdateSource{
					UpdateInfo: "zsync|https://example.com/app.zsync",
					Transport:  "zsync",
				},
			},
		},
	}

	id, replacement, err := ResolveManagedAppIdentity("New Name", "new.desktop", "/tmp/New.AppImage", incoming, existing)
	if err != nil {
		t.Fatalf("ResolveManagedAppIdentity returned error: %v", err)
	}
	if id != "new-name" {
		t.Fatalf("id = %q, want new-name", id)
	}
	if replacement == nil || replacement.ID != "old-id" {
		t.Fatalf("replacement = %+v, want old-id", replacement)
	}
}

func TestEquivalentManagedAppIgnoresAmbiguousMatches(t *testing.T) {
	incoming := &App{
		Source: Source{Kind: SourceDirectURL, DirectURL: &DirectURLSource{URL: "https://example.com/app.AppImage"}},
	}
	existing := map[string]*App{
		"one": {ID: "one", Source: Source{Kind: SourceDirectURL, DirectURL: &DirectURLSource{URL: "https://example.com/app.AppImage"}}},
		"two": {ID: "two", Source: Source{Kind: SourceDirectURL, DirectURL: &DirectURLSource{URL: "https://example.com/app.AppImage"}}},
	}

	if got := EquivalentManagedApp(incoming, existing, ""); got != nil {
		t.Fatalf("EquivalentManagedApp = %+v, want nil for ambiguous matches", got)
	}
}
