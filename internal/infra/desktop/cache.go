package desktop

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
)

var (
	IntegrationCacheLookPath       = exec.LookPath
	IntegrationCacheCommandContext = exec.CommandContext
	IntegrationCacheWarn           = func(msg string) { fmt.Fprintln(os.Stderr, msg) }
)

type CachePaths struct {
	DesktopDir   string
	IconThemeDir string
}

func RefreshIntegrationCaches(ctx context.Context, paths CachePaths) {
	if _, err := runCacheCommandIfAvailable(ctx, "update-desktop-database", paths.DesktopDir); err != nil {
		IntegrationCacheWarn(fmt.Sprintf("Warning: failed to refresh desktop database: %v", err))
	}

	if err := refreshKDEServiceCache(ctx); err != nil {
		IntegrationCacheWarn(fmt.Sprintf("Warning: failed to refresh KDE service cache: %v", err))
	}

	ranXDG, err := runCacheCommandIfAvailable(ctx, "xdg-icon-resource", "forceupdate")
	if err != nil {
		IntegrationCacheWarn(fmt.Sprintf("Warning: failed to refresh icon cache via xdg-icon-resource: %v", err))
		return
	}
	if ranXDG {
		return
	}

	if _, err := runCacheCommandIfAvailable(ctx, "gtk-update-icon-cache", "-f", paths.IconThemeDir); err != nil {
		IntegrationCacheWarn(fmt.Sprintf("Warning: failed to refresh icon cache via gtk-update-icon-cache: %v", err))
	}
}

func refreshKDEServiceCache(ctx context.Context) error {
	if ran, err := runCacheCommandIfAvailable(ctx, "kbuildsycoca6"); ran || err != nil {
		return err
	}
	if _, err := runCacheCommandIfAvailable(ctx, "kbuildsycoca5"); err != nil {
		return err
	}
	return nil
}

func runCacheCommandIfAvailable(ctx context.Context, name string, args ...string) (bool, error) {
	binary, err := IntegrationCacheLookPath(name)
	if err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return false, nil
		}
		return false, err
	}
	if err := IntegrationCacheCommandContext(ctx, binary, args...).Run(); err != nil {
		return true, err
	}
	return true, nil
}
