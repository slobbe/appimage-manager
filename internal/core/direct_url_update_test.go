package core

import (
	"testing"

	models "github.com/slobbe/appimage-manager/internal/types"
)

func TestDirectURLUpdateCheck(t *testing.T) {
	update, err := DirectURLUpdateCheck(&models.UpdateSource{
		Kind: models.UpdateDirectURL,
		DirectURL: &models.DirectURLUpdateSource{
			URL:    "https://example.com/MyApp.AppImage",
			SHA256: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		},
	}, "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")
	if err != nil {
		t.Fatalf("DirectURLUpdateCheck returned error: %v", err)
	}
	if update == nil {
		t.Fatal("expected update response")
	}
	if !update.Available {
		t.Fatal("expected update to be available")
	}
}

func TestDirectURLUpdateCheckNoUpdateWhenSHA256Matches(t *testing.T) {
	update, err := DirectURLUpdateCheck(&models.UpdateSource{
		Kind: models.UpdateDirectURL,
		DirectURL: &models.DirectURLUpdateSource{
			URL:    "https://example.com/MyApp.AppImage",
			SHA256: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		},
	}, "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	if err != nil {
		t.Fatalf("DirectURLUpdateCheck returned error: %v", err)
	}
	if update.Available {
		t.Fatal("expected no update when sha256 matches")
	}
}

func TestDirectURLUpdateCheckRequiresHTTPSAndSHA256(t *testing.T) {
	_, err := DirectURLUpdateCheck(&models.UpdateSource{
		Kind: models.UpdateDirectURL,
		DirectURL: &models.DirectURLUpdateSource{
			URL:    "http://example.com/MyApp.AppImage",
			SHA256: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		},
	}, "")
	if err == nil {
		t.Fatal("expected error for non-https url")
	}

	_, err = DirectURLUpdateCheck(&models.UpdateSource{
		Kind: models.UpdateDirectURL,
		DirectURL: &models.DirectURLUpdateSource{
			URL:    "https://example.com/MyApp.AppImage",
			SHA256: "abc",
		},
	}, "")
	if err == nil {
		t.Fatal("expected error for invalid sha256")
	}
}
