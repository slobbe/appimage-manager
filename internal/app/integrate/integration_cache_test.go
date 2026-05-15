package integrate

import (
	"context"
	"testing"
)

func TestRefreshDesktopIntegrationCachesDelegatesConfiguredPaths(t *testing.T) {
	originalPaths := defaultPaths
	t.Cleanup(func() {
		defaultPaths = originalPaths
	})
	SetPaths(Paths{
		AimDir:       "/tmp/aim",
		DesktopDir:   "/tmp/applications",
		TempDir:      "/tmp/tmp",
		IconThemeDir: "/tmp/hicolor",
	})
	originalRefresher := defaultDesktopIntegrationCacheRefresher
	t.Cleanup(func() {
		defaultDesktopIntegrationCacheRefresher = originalRefresher
	})

	refresher := &recordingIntegrationCacheRefresher{}
	SetDesktopIntegrationCacheRefresher(refresher)

	refreshDesktopIntegrationCaches(context.Background())

	if refresher.desktopDir != "/tmp/applications" {
		t.Fatalf("desktopDir = %q, want /tmp/applications", refresher.desktopDir)
	}
	if refresher.iconThemeDir != "/tmp/hicolor" {
		t.Fatalf("iconThemeDir = %q, want /tmp/hicolor", refresher.iconThemeDir)
	}
}

type recordingIntegrationCacheRefresher struct {
	desktopDir   string
	iconThemeDir string
}

func (refresher *recordingIntegrationCacheRefresher) RefreshDesktopIntegrationCaches(ctx context.Context, desktopDir, iconThemeDir string) {
	refresher.desktopDir = desktopDir
	refresher.iconThemeDir = iconThemeDir
}
