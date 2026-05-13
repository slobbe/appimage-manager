package app

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/slobbe/appimage-manager/internal/cli/config"
	"github.com/slobbe/appimage-manager/internal/infra/desktop"
	repo "github.com/slobbe/appimage-manager/internal/infra/repository"
)

func setupIntegrationConfigForTest(t *testing.T, tmp string) {
	t.Helper()

	originalAimDir := config.AimDir
	originalTempDir := config.TempDir
	originalDesktopDir := config.DesktopDir
	originalIconThemeDir := config.IconThemeDir
	originalDbSrc := config.DbSrc
	originalStore := defaultStore
	originalPaths := defaultPaths
	t.Cleanup(func() {
		config.AimDir = originalAimDir
		config.TempDir = originalTempDir
		config.DesktopDir = originalDesktopDir
		config.IconThemeDir = originalIconThemeDir
		config.DbSrc = originalDbSrc
		defaultStore = originalStore
		defaultPaths = originalPaths
	})

	config.AimDir = filepath.Join(tmp, "aim")
	config.TempDir = filepath.Join(tmp, "cache", "tmp")
	config.DesktopDir = filepath.Join(tmp, "applications")
	config.IconThemeDir = filepath.Join(tmp, "icons", "hicolor")
	config.DbSrc = filepath.Join(tmp, "state", "aim", "apps.json")
	SetPaths(Paths{
		AimDir:       config.AimDir,
		DesktopDir:   config.DesktopDir,
		TempDir:      config.TempDir,
		IconThemeDir: config.IconThemeDir,
	})
	SetStore(repo.NewStore(config.DbSrc))

	for _, dir := range []string{config.AimDir, config.DesktopDir, config.IconThemeDir, filepath.Dir(config.DbSrc)} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("failed to create test dir %q: %v", dir, err)
		}
	}
}

func stubIntegrationCacheRefreshForTest(t *testing.T) {
	t.Helper()

	originalLookPath := desktop.IntegrationCacheLookPath
	originalWarn := desktop.IntegrationCacheWarn
	t.Cleanup(func() {
		desktop.IntegrationCacheLookPath = originalLookPath
		desktop.IntegrationCacheWarn = originalWarn
	})

	desktop.IntegrationCacheLookPath = func(string) (string, error) {
		return "", exec.ErrNotFound
	}
	desktop.IntegrationCacheWarn = func(string) {}
}
