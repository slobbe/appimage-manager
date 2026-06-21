package app

import "context"

// AppImageExtractor extracts AppImages into a workspace directory.
type AppImageExtractor interface {
	Extract(ctx context.Context, appImagePath string, destDir string) (AppImageExtraction, error)
}

// AppImageExtraction describes an extracted AppImage filesystem.
type AppImageExtraction struct {
	RootDir    string
	UpdateInfo string
}

// AppImageStager copies a user-provided AppImage into a workspace before extraction.
type AppImageStager interface {
	Stage(ctx context.Context, sourcePath string, workspacePath string) (string, error)
}

// DesktopEntryDiscoverer finds desktop entry files in extracted AppImages.
type DesktopEntryDiscoverer interface {
	Discover(ctx context.Context, rootDir string) (DesktopEntryFile, error)
}

// DesktopEntryFile is a discovered .desktop file and its contents.
type DesktopEntryFile struct {
	Path    string
	Content []byte
}

// IconDiscoverer finds application icon files in extracted AppImages.
type IconDiscoverer interface {
	Discover(ctx context.Context, rootDir string, iconName string) (IconFile, error)
}

// IconFile is a discovered icon file.
type IconFile struct {
	Path string
}

// AppImageInstaller installs an AppImage into the app library.
type AppImageInstaller interface {
	Install(ctx context.Context, sourcePath string, appID string) (string, error)
}

// ArtifactRemover removes installed files created by aim.
type ArtifactRemover interface {
	Remove(ctx context.Context, path string) error
}

// IconInstaller installs an icon into the icon directory.
type IconInstaller interface {
	Install(ctx context.Context, sourcePath string, appID string) (string, error)
}

// DesktopEntryInstaller installs a desktop entry into the applications directory.
type DesktopEntryInstaller interface {
	Install(ctx context.Context, appID string, content []byte) (string, error)
}

// DesktopIntegrationRefresher refreshes desktop environment caches after
// installing or removing desktop integration artifacts.
type DesktopIntegrationRefresher interface {
	Refresh(ctx context.Context) error
}
